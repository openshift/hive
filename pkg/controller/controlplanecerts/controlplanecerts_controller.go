package controlplanecerts

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName           = "controlPlaneCerts"
	openshiftConfigNamespace = "openshift-config"

	certsNotFoundReason  = "ControlPlaneCertificatesNotFound"
	certsNotFoundMessage = "One or more serving certificates for the cluster control plane are missing"
	certsFoundReason     = "ControlPlaneCertificatesFound"
	certsFoundMessage    = "Control plane certificates are present"

	kubeAPIServerPatchTemplate = `[ {"op": "replace", "path": "/spec/forceRedeploymentReason", "value": %q } ]`
)

var (
	secretCheckInterval = 2 * time.Minute
)

type applier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new ControlPlaneCerts Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	helper := resource.NewHelperWithMetricsFromRESTConfig(mgr.GetConfig(), controllerName, logger)
	return &ReconcileControlPlaneCerts{
		Client:  controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:  mgr.GetScheme(),
		applier: helper,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("controlplanecerts-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileControlPlaneCerts{}

// ReconcileControlPlaneCerts reconciles a ClusterDeployment object
type ReconcileControlPlaneCerts struct {
	client.Client
	scheme  *runtime.Scheme
	applier applier
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
func (r *ReconcileControlPlaneCerts) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
		"controller":        controllerName,
	})

	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	existingSyncSet := &hivev1.SyncSet{}
	existingSyncSetNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controlPlaneCertsSyncSetName(cd.Name)}
	err = r.Get(context.TODO(), existingSyncSetNamespacedName, existingSyncSet)
	if err != nil && !errors.IsNotFound(err) {
		cdLog.WithError(err).Error("failed to retrieve existing control plane certs syncset")
		return reconcile.Result{}, err
	}
	if errors.IsNotFound(err) {
		existingSyncSet = nil
	}

	secrets, secretsAvailable, err := r.getControlPlaneSecrets(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("failed to check cert secret availability")
		return reconcile.Result{}, err
	}

	// clear condition if certs were found
	updated, err := r.setCertsNotFoundCondition(cd, !secretsAvailable, cdLog)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot update cluster deployment secrets not found condition")
		return reconcile.Result{}, err
	}
	if updated {
		return reconcile.Result{}, nil
	}

	if !secretsAvailable {
		cdLog.Debugf("cert secrets are not available yet, requeueing clusterdeployment for %s", secretCheckInterval)
		return reconcile.Result{RequeueAfter: secretCheckInterval}, nil
	}

	if len(secrets) == 0 && existingSyncSet == nil {
		cdLog.Debug("no control plane certs needed, and no syncset exists, nothing to do")
		return reconcile.Result{}, nil
	}

	desiredSyncSet, err := r.generateControlPlaneCertsSyncSet(cd, secrets, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("failed to generate control plane certs syncset")
		return reconcile.Result{}, err
	}

	if _, err = r.applier.ApplyRuntimeObject(desiredSyncSet, r.scheme); err != nil {
		cdLog.WithError(err).Error("failed to apply control plane certificates syncset")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileControlPlaneCerts) getControlPlaneSecrets(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]*corev1.Secret, bool, error) {
	secretsNeeded, err := getControlPlaneSecretNames(cd, cdLog)
	if err != nil {
		return nil, false, err
	}
	if len(secretsNeeded) == 0 {
		cdLog.Debug("the control plane does not require any cert bundles")
		return nil, true, nil
	}
	secrets := []*corev1.Secret{}
	for _, secretName := range secretsNeeded {
		secret := &corev1.Secret{}
		err := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: secretName}, secret)
		if err != nil {
			if errors.IsNotFound(err) {
				cdLog.WithField("secret", secretName).Debug("certificate secret is not available yet, will check later")
				return nil, false, nil
			}
			cdLog.WithError(err).WithField("secret", secretName).Debug("error retrieving certificate secret")
			return nil, false, err
		}
		secrets = append(secrets, secret)
	}

	cdLog.Debug("all required certificate secrets are available")
	return secrets, true, nil
}

func getControlPlaneSecretNames(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]string, error) {
	certs := sets.NewString()
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default != "" {
		certs.Insert(cd.Spec.ControlPlaneConfig.ServingCertificates.Default)
	}

	for _, additional := range cd.Spec.ControlPlaneConfig.ServingCertificates.Additional {
		certs.Insert(additional.Name)
	}
	if certs.Len() == 0 {
		return nil, nil
	}
	cdLog.WithField("certbundles", certs.List()).Debug("cert bundles used by the control plane")

	secretsNeeded := sets.NewString()
	for _, cert := range certs.List() {
		bundle := certificateBundle(cd, cert)
		if bundle == nil {
			// should not happen if clusterdeployment was validated
			return nil, fmt.Errorf("no certificate bundle was found for %s", cert)
		}
		secretsNeeded.Insert(bundle.CertificateSecretRef.Name)
	}
	cdLog.WithField("secrets", secretsNeeded.List()).Debug("certificate secrets needed by the control plane")
	return secretsNeeded.List(), nil
}

func (r *ReconcileControlPlaneCerts) generateControlPlaneCertsSyncSet(cd *hivev1.ClusterDeployment, secrets []*corev1.Secret, cdLog log.FieldLogger) (*hivev1.SyncSet, error) {
	cdLog.Debug("generating syncset for control plane secrets")
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controlPlaneCertsSyncSetName(cd.Name),
			Namespace: cd.Namespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.SyncResourceApplyMode,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cd.Name,
				},
			},
		},
	}

	// Using SecretReference to sync secrets
	secretReferences := []hivev1.SecretReference{}
	for _, secret := range secrets {
		cdLog.WithField("secret", secret.Name).Debug("adding secret to secretReferences list")
		secretReference := hivev1.SecretReference{
			Source: corev1.ObjectReference{
				Namespace: secret.Namespace,
				Name:      secret.Name,
			},
			Target: corev1.ObjectReference{
				Name:      remoteSecretName(secret.Name, cd),
				Namespace: openshiftConfigNamespace,
			},
		}
		secretReferences = append(secretReferences, secretReference)
	}
	syncSet.Spec.SecretReferences = secretReferences

	resources := []runtime.RawExtension{}
	apiServerConfig := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
	}
	additionalCerts := cd.Spec.ControlPlaneConfig.ServingCertificates.Additional
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default != "" {
		cdLog.Debug("setting default serving certificate for control plane")
		cpCert := hivev1.ControlPlaneAdditionalCertificate{
			Name:   cd.Spec.ControlPlaneConfig.ServingCertificates.Default,
			Domain: defaultControlPlaneDomain(cd),
		}
		additionalCerts = append([]hivev1.ControlPlaneAdditionalCertificate{cpCert}, additionalCerts...)
	}
	for _, additional := range additionalCerts {
		cdLog.WithField("name", additional.Name).Debug("adding named certificate to control plane config")
		bundle := certificateBundle(cd, additional.Name)
		apiServerConfig.Spec.ServingCerts.NamedCertificates = append(apiServerConfig.Spec.ServingCerts.NamedCertificates, configv1.APIServerNamedServingCert{
			Names: []string{additional.Domain},
			ServingCertificate: configv1.SecretNameReference{
				Name: remoteSecretName(bundle.CertificateSecretRef.Name, cd),
			},
		})
	}
	resources = append(resources, runtime.RawExtension{Object: apiServerConfig})

	var err error
	resources, err = controllerutils.AddTypeMeta(resources, r.scheme)
	if err != nil {
		cdLog.WithError(err).Error("cannot add typemeta to syncset resources")
		return nil, err
	}
	// kubeAPIServerPatch sets the forceRedeploymentField on the kube API server cluster operator
	// to a hash of all the cert secrets. If the content of the certs secrets changes, then the new
	// hash value will force the kube API server to redeploy and apply the new certs.
	kubeAPIServerPatch := hivev1.SyncObjectPatch{
		APIVersion: "operator.openshift.io/v1",
		Kind:       "KubeAPIServer",
		Name:       "cluster",
		Patch:      fmt.Sprintf(kubeAPIServerPatchTemplate, secretsHash(secrets)),
		PatchType:  "json",
	}
	syncSet.Spec.Resources = resources
	syncSet.Spec.Patches = []hivev1.SyncObjectPatch{kubeAPIServerPatch}

	// ensure the syncset gets cleaned up when the clusterdeployment is deleted
	if err := controllerutil.SetControllerReference(cd, syncSet, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting owner reference")
		return nil, err
	}

	return syncSet, nil
}

func (r *ReconcileControlPlaneCerts) setCertsNotFoundCondition(cd *hivev1.ClusterDeployment, notFound bool, cdLog log.FieldLogger) (bool, error) {
	status := corev1.ConditionFalse
	reason := certsFoundReason
	message := certsFoundMessage
	if notFound {
		status = corev1.ConditionTrue
		reason = certsNotFoundReason
		message = certsNotFoundMessage
	}

	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ControlPlaneCertificateNotFoundCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever,
	)

	if !changed {
		return false, nil
	}

	cd.Status.Conditions = conds
	return true, r.Status().Update(context.TODO(), cd)
}

func remoteSecretName(secretName string, cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, secretName)
}

func certificateBundle(cd *hivev1.ClusterDeployment, name string) *hivev1.CertificateBundleSpec {
	for i, bundle := range cd.Spec.CertificateBundles {
		if bundle.Name == name {
			return &cd.Spec.CertificateBundles[i]
		}
	}
	return nil
}

func controlPlaneCertsSyncSetName(name string) string {
	return apihelpers.GetResourceName(name, "cp-certs")
}

func defaultControlPlaneDomain(cd *hivev1.ClusterDeployment) string {
	return fmt.Sprintf("api.%s.%s", cd.Spec.ClusterName, cd.Spec.BaseDomain)
}

func writeSecretData(w io.Writer, secret *corev1.Secret) {
	// sort secret keys so we get a repeatable hash
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.StringSlice(keys).Sort()
	fmt.Fprintf(w, "%s/%s\n", secret.Namespace, secret.Name)
	for _, k := range keys {
		fmt.Fprintf(w, "%s: %x\n", k, secret.Data[k])
	}
}

func secretsHash(secrets []*corev1.Secret) string {
	// sort secrets by name so we get a repeatable hash
	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})
	hashWriter := md5.New()
	for _, secret := range secrets {
		writeSecretData(hashWriter, secret)
	}
	return fmt.Sprintf("%x", hashWriter.Sum(nil))
}

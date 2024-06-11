package controlplanecerts

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/resource"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
)

const (
	ControllerName           = hivev1.ControlPlaneCertsControllerName
	openshiftConfigNamespace = "openshift-config"

	certsNotFoundReason  = "ControlPlaneCertificatesNotFound"
	certsNotFoundMessage = "One or more serving certificates for the cluster control plane are missing"
	certsFoundReason     = "ControlPlaneCertificatesFound"
	certsFoundMessage    = "Control plane certificates are present"

	kubeAPIServerPatchTemplate = `[ {"op": "replace", "path": "/spec/forceRedeploymentReason", "value": %q } ]`
)

var (
	secretCheckInterval = 2 * time.Minute

	// clusterDeploymentControlPlaneCertsConditions are the cluster deployment conditions controlled by
	// Control Plane Certs controller
	clusterDeploymentControlPlaneCertsConditions = []hivev1.ClusterDeploymentConditionType{
		hivev1.ControlPlaneCertificateNotFoundCondition,
	}
)

type applier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new ControlPlaneCerts Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	logger := log.WithField("controller", ControllerName)
	helper, err := resource.NewHelperWithMetricsFromRESTConfig(mgr.GetConfig(), ControllerName, logger)
	if err != nil {
		// Hard exit if we can't create this controller
		logger.WithError(err).Fatal("unable to create resource helper")
	}
	r := &ReconcileControlPlaneCerts{
		Client:  controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:  mgr.GetScheme(),
		applier: helper,
	}

	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("controlplanecerts-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), &handler.EnqueueRequestForObject{})
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
func (r *ReconcileControlPlaneCerts) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentControlPlaneCertsConditions)
	if changed {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing control plane certs controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Ensure owner references are correctly set
	err = controllerutils.ReconcileOwnerReferences(cd, generateOwnershipUniqueKeys(cd), r, r.scheme, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("Error reconciling object ownership")
		return reconcile.Result{}, err
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		return reconcile.Result{}, nil
	}

	existingSyncSet := &hivev1.SyncSet{}
	existingSyncSetNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: GenerateControlPlaneCertsSyncSetName(cd.Name)}
	err = r.Get(context.TODO(), existingSyncSetNamespacedName, existingSyncSet)
	if err != nil && !apierrors.IsNotFound(err) {
		cdLog.WithError(err).Error("failed to retrieve existing control plane certs syncset")
		return reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) {
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
			if apierrors.IsNotFound(err) {
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
			Name:        GenerateControlPlaneCertsSyncSetName(cd.Name),
			Namespace:   cd.Namespace,
			Annotations: map[string]string{constants.SyncSetMetricsGroupAnnotation: "cp-certs"},
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				ResourceApplyMode: hivev1.UpsertResourceApplyMode,
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cd.Name,
				},
			},
		},
	}

	// Using SecretMapping to sync secrets
	secretMappings := []hivev1.SecretMapping{}
	for _, secret := range secrets {
		cdLog.WithField("secret", secret.Name).Debug("adding secret to secretMappings list")
		secretMapping := hivev1.SecretMapping{
			SourceRef: hivev1.SecretReference{
				Namespace: secret.Namespace,
				Name:      secret.Name,
			},
			TargetRef: hivev1.SecretReference{
				Name:      remoteSecretName(secret.Name, cd),
				Namespace: openshiftConfigNamespace,
			},
		}
		secretMappings = append(secretMappings, secretMapping)
	}
	syncSet.Spec.Secrets = secretMappings

	servingCertsPatchStr, err := r.getServingCertificatesJSONPatch(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("error building serving certificates JSON patch")
		return nil, err
	}
	cdLog.Debugf("build serving certs patch: %s", servingCertsPatchStr)
	servingCertsPatch := hivev1.SyncObjectPatch{
		APIVersion: "config.openshift.io/v1",
		Kind:       "APIServer",
		Name:       "cluster",
		Patch:      servingCertsPatchStr,
		PatchType:  "json",
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
	syncSet.Spec.Patches = []hivev1.SyncObjectPatch{servingCertsPatch, kubeAPIServerPatch}

	// ensure the syncset gets cleaned up when the clusterdeployment is deleted
	cdLog.WithField("derivedObject", syncSet.Name).Debug("Setting labels on derived object")
	syncSet.Labels = k8slabels.AddLabel(syncSet.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
	syncSet.Labels = k8slabels.AddLabel(syncSet.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeControlPlaneCerts)
	if err := controllerutil.SetControllerReference(cd, syncSet, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting owner reference")
		return nil, err
	}

	return syncSet, nil
}

func (r *ReconcileControlPlaneCerts) getServingCertificatesJSONPatch(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (string, error) {
	var buf strings.Builder

	additionalCerts := cd.Spec.ControlPlaneConfig.ServingCertificates.Additional
	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default != "" {
		cdLog.Debug("setting default serving certificate for control plane")

		apidomain, err := r.defaultControlPlaneDomain(cd)
		if err != nil {
			cdLog.WithError(err).Error("failed to get control plane domain")
			return "", err
		}

		cpCert := hivev1.ControlPlaneAdditionalCertificate{
			Name:   cd.Spec.ControlPlaneConfig.ServingCertificates.Default,
			Domain: apidomain,
		}
		additionalCerts = append([]hivev1.ControlPlaneAdditionalCertificate{cpCert}, additionalCerts...)
	}
	for i, additional := range additionalCerts {
		if i > 0 {
			buf.WriteString(",")
		}
		bundle := certificateBundle(cd, additional.Name)
		cdLog.WithField("name", additional.Name).Debug("adding named certificate to control plane config")
		buf.WriteString(fmt.Sprintf(` { "names": [ "%s" ], "servingCertificate": { "name": "%s" } }`,
			additional.Domain, remoteSecretName(bundle.CertificateSecretRef.Name, cd)))
	}

	var kubeAPIServerNamedCertsTemplate = `[ { "op": "add", "path": "/spec/servingCerts", "value": {} }, { "op": "add", "path": "/spec/servingCerts/namedCertificates", "value": [  ] }, { "op": "replace", "path": "/spec/servingCerts/namedCertificates", "value": [ %s ] } ]`
	namedCerts := buf.String()
	return fmt.Sprintf(kubeAPIServerNamedCertsTemplate, namedCerts), nil

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

// defaultControlPlaneDomain will attempt to return the domain/hostname for the secondary API URL
// for the cluster based on the contents of the clusterDeployment's adminKubeConfig secret.
func (r *ReconcileControlPlaneCerts) defaultControlPlaneDomain(cd *hivev1.ClusterDeployment) (string, error) {
	apiurl, err := remoteclient.InitialURL(r.Client, cd)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch initial API URL")
	}

	u, err := url.Parse(apiurl)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse cluster's API URL")
	}
	return u.Hostname(), nil
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

// GenerateControlPlaneCertsSyncSetName generates the name of the SyncSet that holds the control plane certificates to sync.
func GenerateControlPlaneCertsSyncSetName(name string) string {
	return apihelpers.GetResourceName(name, constants.ControlPlaneCertificateSuffix)
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

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList: &hivev1.SyncSetList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SyncSetTypeLabel:           constants.SyncSetTypeControlPlaneCerts,
			},
			Controlled: true,
		},
	}
}

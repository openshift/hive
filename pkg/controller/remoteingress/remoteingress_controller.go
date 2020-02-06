package remoteingress

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"sort"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	ingresscontroller "github.com/openshift/api/operator/v1"
	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName = "remoteingress"

	// namespace where the ingressController objects must be created
	remoteIngressControllerNamespace = "openshift-ingress-operator"

	// while the IngressController objects live in openshift-ingress-operator
	// the secrets that the ingressControllers refer to must live in openshift-ingress
	remoteIngressControllerSecretsNamespace = "openshift-ingress"

	ingressCertificateNotFoundReason = "IngressCertificateNotFound"
	ingressCertificateFoundReason    = "IngressCertificateFound"

	ingressSecretTolerationKey = "hive.openshift.io/ingress"

	// requeueAfter2 is just a static 2 minute delay for when to requeue
	// for the case when a necessary secret is missing
	requeueAfter2 = time.Minute * 2
)

// kubeCLIApplier knows how to ApplyRuntimeObject.
type kubeCLIApplier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	helper := resource.NewHelperWithMetricsFromRESTConfig(mgr.GetConfig(), controllerName, logger)
	return &ReconcileRemoteClusterIngress{
		Client:  utils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:  mgr.GetScheme(),
		logger:  log.WithField("controller", controllerName),
		kubeCLI: helper,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("remoteingress-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: utils.GetConcurrentReconciles()})
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

type reconcileContext struct {
	clusterDeployment *hivev1.ClusterDeployment
	certBundleSecrets []*corev1.Secret
	logger            log.FieldLogger
}

var _ reconcile.Reconciler = &ReconcileRemoteClusterIngress{}

// ReconcileRemoteClusterIngress reconciles the ingress objects defined in a ClusterDeployment object
type ReconcileRemoteClusterIngress struct {
	client.Client
	scheme  *runtime.Scheme
	kubeCLI kubeCLIApplier

	logger log.FieldLogger
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and sets up
// any needed ClusterIngress objects up for syncing to the remote cluster.
//
func (r *ReconcileRemoteClusterIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
	})
	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	rContext := &reconcileContext{}

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found (must have been deleted), return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}
	rContext.clusterDeployment = cd

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	rContext.logger = cdLog

	if len(cd.Spec.Ingress) == 0 {
		// the admission controller will ensure that we get valid-looking
		// Spec.Ingress (ie no missing 'default', no going from a defined
		// ingress list to an empty list, etc)
		rContext.logger.Debug("no ingress objects defined. using default intaller behavior.")
		return reconcile.Result{}, nil
	}

	// can't proceed if the secret(s) referred to doesn't exist
	certBundleSecrets, err := r.getIngressSecrets(rContext)
	if err != nil {
		rContext.logger.Warningf("will need to retry until able to find all certBundle secrets : %v", err)
		conditionErr := r.setIngressCertificateNotFoundCondition(rContext, true, err.Error())
		if conditionErr != nil {
			rContext.logger.WithError(conditionErr).Error("unable to set IngressCertNotFound condition")
			return reconcile.Result{}, conditionErr
		}

		// no error return b/c we just need to wait for the certificate/secret to appear
		// which is out of our control.
		return reconcile.Result{
			Requeue:      true,
			RequeueAfter: requeueAfter2,
		}, nil
	}
	if err := r.setIngressCertificateNotFoundCondition(rContext, false, ""); err != nil {
		rContext.logger.WithError(err).Error("error setting clusterDeployment condition")
		return reconcile.Result{}, err
	}

	rContext.certBundleSecrets = certBundleSecrets

	if err := r.syncClusterIngress(rContext); err != nil {
		cdLog.Errorf("error syncing clusterIngress syncset: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// syncClusterIngress will create the syncSet with all the needed secrets and
// ingressController objects to sync to the remote cluster
func (r *ReconcileRemoteClusterIngress) syncClusterIngress(rContext *reconcileContext) error {
	rContext.logger.Info("reconciling ClusterIngress for cluster deployment")

	rawList := rawExtensionsFromClusterDeployment(rContext)
	secretMappings := secretMappingsFromClusterDeployment(rContext)
	return r.syncSyncSet(rContext, rawList, secretMappings)
}

// rawExtensionsFromClusterDeployment will return the slice of runtime.RawExtension objects
// (really the syncSet.Spec.Resources) to satisfy the ingress config for the clusterDeployment
func rawExtensionsFromClusterDeployment(rContext *reconcileContext) []runtime.RawExtension {
	rawList := []runtime.RawExtension{}

	for _, ingress := range rContext.clusterDeployment.Spec.Ingress {
		ingressObj := createIngressController(rContext.clusterDeployment, ingress, rContext.certBundleSecrets)
		raw := runtime.RawExtension{Object: ingressObj}
		rawList = append(rawList, raw)
	}

	return rawList
}

// secretMappingsFromClusterDeployment will return the slice of hivev1.SecretMapping objects
// (really the syncSet.Spec.SecretMappings) to satisfy the ingress config for the clusterDeployment
func secretMappingsFromClusterDeployment(rContext *reconcileContext) []hivev1.SecretMapping {
	secretMappings := []hivev1.SecretMapping{}

	for _, cbSecret := range rContext.certBundleSecrets {
		secretMapping := hivev1.SecretMapping{
			SourceRef: hivev1.SecretReference{
				Namespace: cbSecret.Namespace,
				Name:      cbSecret.Name,
			},
			TargetRef: hivev1.SecretReference{
				Namespace: remoteIngressControllerSecretsNamespace,
				Name:      remoteSecretNameForCertificateBundleSecret(cbSecret.Name, rContext.clusterDeployment),
			},
		}
		secretMappings = append(secretMappings, secretMapping)
	}
	return secretMappings
}

func newSyncSetSpec(cd *hivev1.ClusterDeployment, rawExtensions []runtime.RawExtension, secretMappings []hivev1.SecretMapping) *hivev1.SyncSetSpec {
	ssSpec := &hivev1.SyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Resources:         rawExtensions,
			Secrets:           secretMappings,
			ResourceApplyMode: "sync",
		},
		ClusterDeploymentRefs: []corev1.LocalObjectReference{
			{
				Name: cd.Name,
			},
		},
	}
	return ssSpec
}

// syncSyncSet builds up a syncSet object with the passed-in rawExtensions as the spec.Resources
func (r *ReconcileRemoteClusterIngress) syncSyncSet(rContext *reconcileContext, rawExtensions []runtime.RawExtension, secretMappings []hivev1.SecretMapping) error {
	ssName := apihelpers.GetResourceName(rContext.clusterDeployment.Name, "clusteringress")

	newSyncSetSpec := newSyncSetSpec(rContext.clusterDeployment, rawExtensions, secretMappings)
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName,
			Namespace: rContext.clusterDeployment.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "SyncSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		Spec: *newSyncSetSpec,
	}

	// ensure the syncset gets cleaned up when the clusterdeployment is deleted
	r.logger.WithField("derivedObject", syncSet.Name).Debug("Setting labels on derived object")
	syncSet.Labels = k8slabels.AddLabel(syncSet.Labels, constants.ClusterDeploymentNameLabel, rContext.clusterDeployment.Name)
	syncSet.Labels = k8slabels.AddLabel(syncSet.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeRemoteIngress)
	if err := controllerutil.SetControllerReference(rContext.clusterDeployment, syncSet, r.scheme); err != nil {
		r.logger.WithError(err).Error("error setting owner reference")
		return err
	}

	if _, err := r.kubeCLI.ApplyRuntimeObject(syncSet, r.scheme); err != nil {
		rContext.logger.WithError(err).Error("failed to apply syncset")
		return err
	}

	return nil
}

// createIngressController will return an ingressController based on a clusterDeployment's
// spec.Ingress object
func createIngressController(cd *hivev1.ClusterDeployment, ingress hivev1.ClusterIngress, secrets []*corev1.Secret) *ingresscontroller.IngressController {
	newIngress := ingresscontroller.IngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       "IngressController",
			APIVersion: ingresscontroller.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingress.Name,
			Namespace: remoteIngressControllerNamespace,
		},
		Spec: ingresscontroller.IngressControllerSpec{
			Domain:            ingress.Domain,
			RouteSelector:     ingress.RouteSelector,
			NamespaceSelector: ingress.NamespaceSelector,
		},
	}

	// if the ingress entry references a certBundle, make sure to put the appropriate looking
	// entry in the ingressController object
	if ingress.ServingCertificate != "" {
		var secretName string
		for _, cb := range cd.Spec.CertificateBundles {
			// assume we're going to find the certBundle as we would've errored earlier
			if cb.Name == ingress.ServingCertificate {
				secretName = cb.CertificateSecretRef.Name
				newIngress.Spec.DefaultCertificate = &corev1.LocalObjectReference{
					Name: remoteSecretNameForCertificateBundleSecret(cb.CertificateSecretRef.Name, cd),
				}
				break
			}
		}

		// NOTE: This toleration is added to cause a reload of the
		// IngressController when the certificate secrets are updated.
		// In the future, this should not be necessary.
		if len(secretName) != 0 {
			newIngress.Spec.NodePlacement = &ingresscontroller.NodePlacement{
				Tolerations: []corev1.Toleration{
					{
						Key:      ingressSecretTolerationKey,
						Operator: corev1.TolerationOpEqual,
						Value:    secretHash(findSecret(secretName, secrets)),
					},
				},
			}
		}
	}

	return &newIngress
}

func (r *ReconcileRemoteClusterIngress) getIngressSecrets(rContext *reconcileContext) ([]*corev1.Secret, error) {
	certSet := sets.NewString()

	for _, ingress := range rContext.clusterDeployment.Spec.Ingress {
		if ingress.ServingCertificate != "" {
			certSet.Insert(ingress.ServingCertificate)
		}
	}

	cbSecrets := []*corev1.Secret{}

	for _, cert := range certSet.List() {
		foundCertBundle := false
		for _, cb := range rContext.clusterDeployment.Spec.CertificateBundles {
			if cb.Name == cert {
				foundCertBundle = true
				cbSecret := &corev1.Secret{}
				searchKey := types.NamespacedName{
					Name:      cb.CertificateSecretRef.Name,
					Namespace: rContext.clusterDeployment.Namespace,
				}

				if err := r.Get(context.TODO(), searchKey, cbSecret); err != nil {
					if errors.IsNotFound(err) {
						return cbSecrets, fmt.Errorf("secret %v for certbundle %v was not found", cb.CertificateSecretRef.Name, cb.Name)
					}
					rContext.logger.WithError(err).Error("error while gathering certBundle secret")
					return cbSecrets, err
				}

				cbSecrets = append(cbSecrets, cbSecret)
			}
		}
		if !foundCertBundle {
			return cbSecrets, fmt.Errorf("didn't find expected certbundle %v", cert)
		}
	}

	return cbSecrets, nil
}

// setIngressCertificateNotFoundCondition will set/unset the condition indicating whether all certificates required
// by the clusterDeployment ingress objects were found. Returns any error encountered while setting the condition.
func (r *ReconcileRemoteClusterIngress) setIngressCertificateNotFoundCondition(rContext *reconcileContext, notFound bool, missingSecretMessage string) error {
	var (
		msg, reason string
		status      corev1.ConditionStatus
		updateCheck utils.UpdateConditionCheck
	)

	origCD := rContext.clusterDeployment.DeepCopy()

	if notFound {
		msg = missingSecretMessage
		status = corev1.ConditionTrue
		reason = ingressCertificateNotFoundReason
		updateCheck = utils.UpdateConditionIfReasonOrMessageChange
	} else {
		msg = fmt.Sprintf("all secrets for ingress found")
		status = corev1.ConditionFalse
		reason = ingressCertificateFoundReason
		updateCheck = utils.UpdateConditionNever
	}

	rContext.clusterDeployment.Status.Conditions = utils.SetClusterDeploymentCondition(rContext.clusterDeployment.Status.Conditions,
		hivev1.IngressCertificateNotFoundCondition, status, reason, msg, updateCheck)

	if !reflect.DeepEqual(rContext.clusterDeployment.Status.Conditions, origCD.Status.Conditions) {
		if err := r.Status().Update(context.TODO(), rContext.clusterDeployment); err != nil {
			rContext.logger.WithError(err).Log(utils.LogLevel(err), "error updating clusterDeployment condition")
			return err
		}
	}

	return nil
}

// remoteSecretNameForCertificateBundleSecret just stitches together a secret name consisting of
// the original certificateBundle's secret name pre-pended with the clusterDeployment.Name
func remoteSecretNameForCertificateBundleSecret(secretName string, cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, secretName)
}

func findSecret(secretName string, secrets []*corev1.Secret) *corev1.Secret {
	for i, s := range secrets {
		if s.Name == secretName {
			return secrets[i]
		}
	}
	return nil
}

func secretHash(secret *corev1.Secret) string {
	if secret == nil {
		return ""
	}

	b := &bytes.Buffer{}
	// Write out map in sorted key order so we
	// can get repeatable hashes
	keys := []string{}
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.Write([]byte(k + ":"))
		b.Write(secret.Data[k])
		b.Write([]byte("\n"))
	}
	return fmt.Sprintf("%x", md5.Sum(b.Bytes()))
}

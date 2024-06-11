package remoteingress

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	k8slabels "github.com/openshift/hive/pkg/util/labels"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	ingresscontroller "github.com/openshift/api/operator/v1"
	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName = hivev1.RemoteIngressControllerName

	// namespace where the ingressController objects must be created
	remoteIngressControllerNamespace = "openshift-ingress-operator"

	// while the IngressController objects live in openshift-ingress-operator
	// the secrets that the ingressControllers refer to must live in openshift-ingress
	remoteIngressControllerSecretsNamespace = "openshift-ingress"

	ingressCertificateNotFoundReason = "IngressCertificateNotFound"
	ingressCertificateFoundReason    = "IngressCertificateFound"

	// requeueAfter2 is just a static 2 minute delay for when to requeue
	// for the case when a necessary secret is missing
	requeueAfter2 = time.Minute * 2
)

// clusterDeploymentRemoteIngressConditions are the cluster deployment conditions controlled by
// Remote Ingress controller
var clusterDeploymentRemoteIngressConditions = []hivev1.ClusterDeploymentConditionType{
	hivev1.IngressCertificateNotFoundCondition,
}

// kubeCLIApplier knows how to ApplyRuntimeObject.
type kubeCLIApplier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new RemoteIngress Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
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
	return &ReconcileRemoteClusterIngress{
		Client:  controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:  mgr.GetScheme(),
		logger:  log.WithField("controller", ControllerName),
		kubeCLI: helper,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("remoteingress-controller", mgr, controller.Options{
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
func (r *ReconcileRemoteClusterIngress) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

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
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)
	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	rContext.clusterDeployment = cd

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentRemoteIngressConditions)
	if changed {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing remote ingress controller conditions")
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

// GenerateRemoteIngressSyncSetName generates the name of the SyncSet that holds the cluster ingress information to sync.
func GenerateRemoteIngressSyncSetName(clusterDeploymentName string) string {
	return apihelpers.GetResourceName(clusterDeploymentName, constants.ClusterIngressSuffix)
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
			ResourceApplyMode: hivev1.SyncResourceApplyMode,
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
	ssName := GenerateRemoteIngressSyncSetName(rContext.clusterDeployment.Name)

	newSyncSetSpec := newSyncSetSpec(rContext.clusterDeployment, rawExtensions, secretMappings)
	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ssName,
			Namespace:   rContext.clusterDeployment.Namespace,
			Annotations: map[string]string{constants.SyncSetMetricsGroupAnnotation: "ingress"},
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
		for _, cb := range cd.Spec.CertificateBundles {
			// assume we're going to find the certBundle as we would've errored earlier
			if cb.Name == ingress.ServingCertificate {
				newIngress.Spec.DefaultCertificate = &corev1.LocalObjectReference{
					Name: remoteSecretNameForCertificateBundleSecret(cb.CertificateSecretRef.Name, cd),
				}
				break
			}
		}
	}

	if ingress.HttpErrorCodePages != nil {
		newIngress.Spec.HttpErrorCodePages = *ingress.HttpErrorCodePages
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
		updateCheck controllerutils.UpdateConditionCheck
	)

	origCD := rContext.clusterDeployment.DeepCopy()

	if notFound {
		msg = missingSecretMessage
		status = corev1.ConditionTrue
		reason = ingressCertificateNotFoundReason
		updateCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	} else {
		msg = "all secrets for ingress found"
		status = corev1.ConditionFalse
		reason = ingressCertificateFoundReason
		updateCheck = controllerutils.UpdateConditionNever
	}

	rContext.clusterDeployment.Status.Conditions = controllerutils.SetClusterDeploymentCondition(rContext.clusterDeployment.Status.Conditions,
		hivev1.IngressCertificateNotFoundCondition, status, reason, msg, updateCheck)

	if !reflect.DeepEqual(rContext.clusterDeployment.Status.Conditions, origCD.Status.Conditions) {
		if err := r.Status().Update(context.TODO(), rContext.clusterDeployment); err != nil {
			rContext.logger.WithError(err).Log(controllerutils.LogLevel(err), "error updating clusterDeployment condition")
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

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList: &hivev1.SyncSetList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SyncSetTypeLabel:           constants.SyncSetTypeRemoteIngress,
			},
			Controlled: true,
		},
	}
}

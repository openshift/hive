package clusterdeployment

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	routev1 "github.com/openshift/api/route/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	librarygocontroller "github.com/openshift/library-go/pkg/controller"
	"github.com/openshift/library-go/pkg/manifest"
	"github.com/openshift/library-go/pkg/verify"
	"github.com/openshift/library-go/pkg/verify/store/sigstore"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/azure"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/imageset"
	"github.com/openshift/hive/pkg/remoteclient"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterDeployment")

	// clusterDeploymentConditions are the cluster deployment conditions controlled or
	// initialized by cluster deployment controller
	clusterDeploymentConditions = []hivev1.ClusterDeploymentConditionType{
		hivev1.DNSNotReadyCondition,
		hivev1.InstallImagesNotResolvedCondition,
		hivev1.ProvisionFailedCondition,
		hivev1.SyncSetFailedCondition,
		hivev1.InstallLaunchErrorCondition,
		hivev1.DeprovisionLaunchErrorCondition,
		hivev1.ProvisionStoppedCondition,
		hivev1.AuthenticationFailureClusterDeploymentCondition,
		hivev1.RequirementsMetCondition,
		hivev1.ProvisionedCondition,

		// ClusterInstall conditions copied over to cluster deployment
		hivev1.ClusterInstallFailedClusterDeploymentCondition,
		hivev1.ClusterInstallCompletedClusterDeploymentCondition,
		hivev1.ClusterInstallStoppedClusterDeploymentCondition,
		hivev1.ClusterInstallRequirementsMetClusterDeploymentCondition,

		// ImageSet condition initialized by cluster deployment
		hivev1.InstallerImageResolutionFailedCondition,
	}
)

const (
	ControllerName     = hivev1.ClusterDeploymentControllerName
	defaultRequeueTime = 10 * time.Second
	maxProvisions      = 3

	platformAuthFailureReason = "PlatformAuthError"
	platformAuthSuccessReason = "PlatformAuthSuccess"

	clusterImageSetNotFoundReason = "ClusterImageSetNotFound"
	clusterImageSetFoundReason    = "ClusterImageSetFound"

	defaultDNSNotReadyTimeout     = 10 * time.Minute
	dnsNotReadyReason             = "DNSNotReady"
	dnsNotReadyTimedoutReason     = "DNSNotReadyTimedOut"
	dnsUnsupportedPlatformReason  = "DNSUnsupportedPlatform"
	dnsZoneResourceConflictReason = "DNSZoneResourceConflict"
	dnsReadyReason                = "DNSReady"
	dnsReadyAnnotation            = "hive.openshift.io/dnsready"

	installAttemptsLimitReachedReason = "InstallAttemptsLimitReached"
	installOnlyOnceSetReason          = "InstallOnlyOnceSet"
	failureReasonNotListed            = "FailureReasonNotRetryable"
	provisionNotStoppedReason         = "ProvisionNotStopped"

	deleteAfterAnnotation    = "hive.openshift.io/delete-after" // contains a duration after which the cluster should be cleaned up.
	tryInstallOnceAnnotation = "hive.openshift.io/try-install-once"

	regionUnknown     = "unknown"
	injectCABundleKey = "config.openshift.io/inject-trusted-cabundle"
)

// Add creates a new ClusterDeployment controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	// Read the metrics config from hive config and set values for mapClusterTypeLabelToValue, if present
	mConfig, err := hivemetrics.ReadMetricsConfig()
	if err != nil {
		log.WithError(err).Error("error reading metrics config")
		return err
	}
	// Register the metrics. This is done here to ensure we define the metrics with optional label support after we have
	// read the hiveconfig, and we register them only once.
	registerMetrics(mConfig, logger)
	return AddToManager(mgr, NewReconciler(mgr, logger, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger log.FieldLogger, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	r := &ReconcileClusterDeployment{
		Client:                                  controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:                                  mgr.GetScheme(),
		logger:                                  logger,
		expectations:                            controllerutils.NewExpectations(logger),
		watchingClusterInstall:                  map[string]struct{}{},
		validateCredentialsForClusterDeployment: controllerutils.ValidateCredentialsForClusterDeployment,
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}

	protectedDeleteEnvVar := os.Getenv(constants.ProtectedDeleteEnvVar)
	if protectedDelete, err := strconv.ParseBool(protectedDeleteEnvVar); protectedDelete && err == nil {
		logger.Info("Protected Delete enabled")
		r.protectedDelete = true
	}

	verifier, err := LoadReleaseImageVerifier(mgr.GetConfig())
	if err == nil {
		logger.Info("Release Image verification enabled")
		r.releaseImageVerifier = verifier
	} else {
		logger.WithError(err).Error("Release Image verification failed to be configured")
	}

	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	cdReconciler, ok := r.(*ReconcileClusterDeployment)
	if !ok {
		return errors.New("reconciler supplied is not a ReconcileClusterDeployment")
	}

	logger := log.WithField("controller", ControllerName)
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		logger.WithError(err).Error("could not create controller")
		return err
	}

	// Inject watcher to the clusterdeployment reconciler.
	controllerutils.InjectWatcher(cdReconciler, c)

	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &hivev1.ClusterDeployment{},
		clusterInstallIndexFieldName, indexClusterInstall); err != nil {
		logger.WithError(err).Error("Error indexing cluster deployment for cluster install")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		logger.WithError(err).Error("Error watching cluster deployment")
		return err
	}

	// Watch for provisions
	if err := cdReconciler.watchClusterProvisions(mgr, c); err != nil {
		return err
	}

	// Watch for jobs created by a ClusterDeployment:
	err = c.Watch(source.Kind(mgr.GetCache(), &batchv1.Job{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()))
	if err != nil {
		logger.WithError(err).Error("Error watching cluster deployment job")
		return err
	}

	// Watch for pods created by an install job
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Pod{}), handler.EnqueueRequestsFromMapFunc(selectorPodWatchHandler))
	if err != nil {
		logger.WithError(err).Error("Error watching cluster deployment pods")
		return err
	}

	// Watch for deprovision requests created by a ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeprovision{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()))
	if err != nil {
		logger.WithError(err).Error("Error watching deprovision request created by cluster deployment")
		return err
	}

	// Watch for dnszones created by a ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.DNSZone{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner()))
	if err != nil {
		logger.WithError(err).Error("Error watching cluster deployment dnszones")
		return err
	}

	// Watch for changes to ClusterSyncs
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hiveintv1alpha1.ClusterSync{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}),
	); err != nil {
		return errors.Wrap(err, "cannot start watch on ClusterSyncs")
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	manager.Manager
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger

	// watcher allows the reconciler to add new watches
	// at runtime.
	watcher controllerutils.Watcher

	// this is a set of cluster install contracts that are currently
	// being watched. this allows the reconciler to only add Watch for
	// these once.
	watchingClusterInstall map[string]struct{}

	// A TTLCache of clusterprovision creates each clusterdeployment expects to see
	expectations controllerutils.ExpectationsInterface

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder

	// validateCredentialsForClusterDeployment is what this controller will call to validate
	// that the platform creds are good (used for testing)
	validateCredentialsForClusterDeployment func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (bool, error)

	// releaseImageVerifier, if provided, will be used to check an release image before it is executed.
	// Any error will prevent a release image from being accessed.
	releaseImageVerifier verify.Interface

	protectedDelete bool
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
func (r *ReconcileClusterDeployment) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cdLog.Info("cluster deployment Not Found")
			r.expectations.DeleteExpectations(request.NamespacedName.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		cdLog.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Ensure owner references are correctly set
	err = controllerutils.ReconcileOwnerReferences(cd, generateOwnershipUniqueKeys(cd), r, r.scheme, r.logger)
	if err != nil {
		cdLog.WithError(err).Error("Error reconciling object ownership")
		return reconcile.Result{}, err
	}

	// Initialize cluster deployment conditions if not set
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentConditions)
	if changed {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing cluster deployment controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Remove legacy conditions if present:
	legacyConditions := []string{
		"ClusterImageSetNotFound",
	}
	newConditions = []hivev1.ClusterDeploymentCondition{}
	var removedLegacyConditions bool
	for _, c := range cd.Status.Conditions {
		isLegacy := false
		for _, lc := range legacyConditions {
			if string(c.Type) == lc {
				isLegacy = true
				break
			}
		}
		if !isLegacy {
			newConditions = append(newConditions, c)
		} else {
			cdLog.Infof("removing legacy condition: %v", c)
			removedLegacyConditions = true
		}
	}
	if removedLegacyConditions {
		cd.Status.Conditions = newConditions
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	return r.reconcile(request, cd, cdLog)
}

func (r *ReconcileClusterDeployment) SetWatcher(w controllerutils.Watcher) {
	r.watcher = w
}

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList:    &hivev1.ClusterProvisionList{},
			LabelSelector: map[string]string{constants.ClusterDeploymentNameLabel: owner.GetName()},
			Controlled:    true,
		},
		{
			TypeToList: &corev1.PersistentVolumeClaimList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.PVCTypeLabel:               constants.PVCTypeInstallLogs,
			},
			Controlled: true,
		},
		{
			TypeToList: &batchv1.JobList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.JobTypeLabel:               constants.JobTypeImageSet,
			},
			Controlled: true,
		},
		{
			TypeToList: &hivev1.ClusterDeprovisionList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
			},
			Controlled: true,
		},
		{
			TypeToList: &hivev1.DNSZoneList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
			},
			Controlled: true,
		},
		{
			TypeToList: &corev1.SecretList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SecretTypeLabel:            constants.SecretTypeMergedPullSecret,
			},
			Controlled: true,
		},
		{
			TypeToList: &corev1.SecretList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SecretTypeLabel:            constants.SecretTypeKubeConfig,
			},
			Controlled: false,
		},
		{
			TypeToList: &corev1.SecretList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SecretTypeLabel:            constants.SecretTypeKubeAdminCreds,
			},
			Controlled: false,
		},
	}
}

func (r *ReconcileClusterDeployment) addAdditionalKubeconfigCAs(cd *hivev1.ClusterDeployment,
	cdLog log.FieldLogger) error {

	adminKubeconfigSecret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name}, adminKubeconfigSecret); err != nil {
		cdLog.WithError(err).Error("failed to get admin kubeconfig secret")
		return err
	}

	originalSecret := adminKubeconfigSecret.DeepCopy()

	rawData, hasRawData := adminKubeconfigSecret.Data[constants.RawKubeconfigSecretKey]
	if !hasRawData {
		adminKubeconfigSecret.Data[constants.RawKubeconfigSecretKey] = adminKubeconfigSecret.Data[constants.KubeconfigSecretKey]
		rawData = adminKubeconfigSecret.Data[constants.KubeconfigSecretKey]
	}

	var err error
	adminKubeconfigSecret.Data[constants.KubeconfigSecretKey], err = controllerutils.AddAdditionalKubeconfigCAs(rawData)
	if err != nil {
		cdLog.WithError(err).Errorf("error adding additional CAs to admin kubeconfig")
		return err
	}

	if reflect.DeepEqual(originalSecret.Data, adminKubeconfigSecret.Data) {
		cdLog.Debug("secret data has not changed, no need to update")
		return nil
	}

	cdLog.Info("admin kubeconfig has been modified, updating")
	err = r.Update(context.TODO(), adminKubeconfigSecret)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating admin kubeconfig secret")
		return err
	}

	return nil
}

func (r *ReconcileClusterDeployment) reconcile(request reconcile.Request, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (result reconcile.Result, returnErr error) {
	// Set platform label on the ClusterDeployment
	if platform := getClusterPlatform(cd); cd.Labels[hivev1.HiveClusterPlatformLabel] != platform {
		if cd.Labels == nil {
			cd.Labels = make(map[string]string)
		}
		if cd.Labels[hivev1.HiveClusterPlatformLabel] != "" {
			cdLog.Warnf("changing the value of %s from %s to %s", hivev1.HiveClusterPlatformLabel,
				cd.Labels[hivev1.HiveClusterPlatformLabel], platform)
		}
		cd.Labels[hivev1.HiveClusterPlatformLabel] = platform
		err := r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to set cluster platform label")
		}
		return reconcile.Result{}, err
	}

	// Set region label on the ClusterDeployment
	if region := getClusterRegion(cd); cd.Spec.Platform.BareMetal == nil && cd.Spec.Platform.AgentBareMetal == nil &&
		cd.Spec.Platform.None == nil && cd.Labels[hivev1.HiveClusterRegionLabel] != region {

		if cd.Labels == nil {
			cd.Labels = make(map[string]string)
		}
		if cd.Labels[hivev1.HiveClusterRegionLabel] != "" {
			cdLog.Warnf("changing the value of %s from %s to %s", hivev1.HiveClusterRegionLabel,
				cd.Labels[hivev1.HiveClusterRegionLabel], region)
		}
		cd.Labels[hivev1.HiveClusterRegionLabel] = region
		err := r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to set cluster region label")
		}
		return reconcile.Result{}, err
	}

	if cd.Spec.ManageDNS {
		changed, err := r.ensureDNSZonePreserveOnDeleteAndLogAnnotations(cd, cdLog)
		if err != nil {
			return reconcile.Result{}, err
		}
		if changed {
			return reconcile.Result{}, nil
		}
	}

	if err := r.ensureTrustedCABundleConfigMap(cd, cdLog); err != nil {
		cdLog.WithError(err).Error("Failed to create or verify the trusted CA bundle ConfigMap")
		return reconcile.Result{}, err
	}

	// Discover Azure ResourceGroup if applicable
	if r.discoverAzureResourceGroup(cd, cdLog) {
		return reconcile.Result{}, r.Update(context.TODO(), cd)
	}

	if cd.DeletionTimestamp != nil {

		return r.syncDeletedClusterDeployment(cd, cdLog)
	}

	// Check for the delete-after annotation, and if the cluster has expired, delete it
	deleteAfter, ok := cd.Annotations[deleteAfterAnnotation]
	if ok {
		cdLog.Debugf("found delete after annotation: %s", deleteAfter)
		dur, err := time.ParseDuration(deleteAfter)
		if err != nil {
			cdLog.WithError(err).WithField("deleteAfter", deleteAfter).Infof("error parsing %s as a duration", deleteAfterAnnotation)
			return reconcile.Result{}, fmt.Errorf("error parsing %s as a duration: %v", deleteAfterAnnotation, err)
		}
		if !cd.CreationTimestamp.IsZero() {
			expiry := cd.CreationTimestamp.Add(dur)
			cdLog.Debugf("cluster expires at: %s", expiry)
			if time.Now().After(expiry) {
				cdLog.WithField("expiry", expiry).Info("cluster has expired, issuing delete")
				err := controllerutils.SafeDelete(r, context.TODO(), cd)
				if err != nil {
					cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting expired cluster")
				}
				return reconcile.Result{}, err
			}

			defer func() {
				// We have an expiry time but we're not expired yet. Set requeueAfter to the expiry time
				// so that we requeue cluster for deletion once reconcile has completed
				result, returnErr = controllerutils.EnsureRequeueAtLeastWithin(
					time.Until(cd.CreationTimestamp.Add(dur)),
					result,
					returnErr,
				)
			}()

		}
	}

	if !controllerutils.HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		if err := r.addClusterDeploymentFinalizer(cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer")
			return reconcile.Result{}, err
		}
		metricClustersCreated.Observe(cd, nil, 1)
		return reconcile.Result{}, nil
	}

	if cd.Spec.ManageDNS {
		updated, result, err := r.ensureManagedDNSZone(cd, cdLog)
		if updated || err != nil {
			return result, err
		}
	}

	if cd.Spec.Installed {
		// set installedTimestamp for adopted clusters
		if cd.Status.InstalledTimestamp == nil {
			cd.Status.InstalledTimestamp = &cd.ObjectMeta.CreationTimestamp
			if err := r.Status().Update(context.TODO(), cd); err != nil {
				cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not set cluster installed timestamp")
				return reconcile.Result{Requeue: true}, nil
			}
		}
		// Set the Provisioned status condition for adopted clusters (and in case we upgraded to/past where that condition was introduced)
		if err := r.updateCondition(
			cd,
			hivev1.ProvisionedCondition,
			corev1.ConditionTrue,
			hivev1.ProvisionedReasonProvisioned,
			"Cluster is provisioned",
			cdLog,
		); err != nil {
			cdLog.WithError(err).Error("Error updating Provisioned status condition")
			return reconcile.Result{}, err
		}

		// update SyncSetFailedCondition status condition
		cdLog.Debug("Check if any syncsets are failing")
		if err := r.setSyncSetFailedCondition(cd, cdLog); err != nil {
			cdLog.WithError(err).Error("Error updating SyncSetFailedCondition status condition")
			return reconcile.Result{}, err
		}

		switch {
		case cd.Spec.Provisioning != nil:
			if r, err := r.reconcileInstalledClusterProvision(cd, cdLog); err != nil {
				return r, err
			}
		}

		if cd.Spec.ClusterMetadata != nil &&
			cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name != "" {

			if err := r.addAdditionalKubeconfigCAs(cd, cdLog); err != nil {
				return reconcile.Result{}, err
			}

			// Add cluster deployment as additional owner reference to admin secrets
			if err := r.addOwnershipToSecret(cd, cdLog, cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name); err != nil {
				return reconcile.Result{}, err
			}
			if cd.Spec.ClusterMetadata.AdminPasswordSecretRef != nil && cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name != "" {
				if err := r.addOwnershipToSecret(cd, cdLog, cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name); err != nil {
					return reconcile.Result{}, err
				}
			}

			if cd.Status.WebConsoleURL == "" || cd.Status.APIURL == "" {
				return r.setClusterStatusURLs(cd, cdLog)
			}

		}
		return reconcile.Result{}, nil
	}

	// If the ClusterDeployment is being relocated to another Hive instance, stop any current provisioning and do not
	// do any more reconciling.
	switch _, relocateStatus, err := controllerutils.IsRelocating(cd); {
	case err != nil:
		return reconcile.Result{}, errors.Wrap(err, "could not determine relocate status")
	case relocateStatus == hivev1.RelocateOutgoing:
		result, err := r.stopProvisioning(cd, cdLog)
		if result == nil {
			result = &reconcile.Result{}
		}
		return *result, err
	}

	// Sanity check the platform/cloud credentials and set hivev1.AuthenticationFailureClusterDeploymentCondition
	validCreds, authError := r.validatePlatformCreds(cd, cdLog)
	_, err := r.setAuthenticationFailure(cd, validCreds, authError, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("unable to update clusterdeployment")
		return reconcile.Result{}, err
	}

	// If the platform credentials are no good, return error and go into backoff
	authCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.AuthenticationFailureClusterDeploymentCondition)
	if authCondition.Status == corev1.ConditionTrue {
		authError := errors.New(authCondition.Message)
		cdLog.WithError(authError).Error("cannot proceed with provision while platform credentials authentication is failing.")
		return reconcile.Result{}, authError
	}

	imageSet, err := r.getClusterImageSet(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("failed to get cluster image set for the clusterdeployment")
		return reconcile.Result{}, err
	}

	releaseImage := r.getReleaseImage(cd, imageSet, cdLog)

	cdLog.Debug("loading pull secrets")
	pullSecret, err := r.mergePullSecrets(cd, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("Error merging pull secrets")
		return reconcile.Result{}, err
	}

	// Update the pull secret object if required
	switch updated, err := r.updatePullSecretInfo(pullSecret, cd, cdLog); {
	case err != nil:
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "Error updating the merged pull secret")
		return reconcile.Result{}, err
	case updated:
		// The controller will not automatically requeue the cluster deployment
		// since the controller is not watching for secrets. So, requeue manually.
		return reconcile.Result{Requeue: true}, nil
	}

	// let's verify the release image before using it here.
	if r.releaseImageVerifier != nil {
		var releaseDigest string
		if index := strings.LastIndex(releaseImage, "@"); index != -1 {
			releaseDigest = releaseImage[index+1:]
		}
		cdLog.WithField("releaseImage", releaseImage).
			WithField("releaseDigest", releaseDigest).Debugf("verifying the release image using %s", r.releaseImageVerifier)

		err := r.releaseImageVerifier.Verify(context.TODO(), releaseDigest)
		if err != nil {
			cdLog.WithField("releaseImage", releaseImage).
				WithField("releaseDigest", releaseDigest).
				WithError(err).Error("Verification of release image failed")
			return reconcile.Result{}, r.updateCondition(cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionTrue, "ReleaseImageVerificationFailed", err.Error(), cdLog)
		}
	}

	switch result, err := r.resolveInstallerImage(cd, releaseImage, cdLog); {
	case err != nil:
		return reconcile.Result{}, err
	case result != nil:
		return *result, nil
	}

	if !r.expectations.SatisfiedExpectations(request.String()) {
		cdLog.Debug("waiting for expectations to be satisfied")
		return reconcile.Result{}, nil
	}

	switch {
	case cd.Spec.Provisioning != nil:
		// Ensure the install config matches the ClusterDeployment:
		// TODO: Move with the openshift-installer ClusterInstall code when we implement https://issues.redhat.com/browse/HIVE-1522
		if cd.Spec.Provisioning.InstallConfigSecretRef == nil {
			cdLog.Info("install config not specified")
			conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				cd.Status.Conditions,
				hivev1.RequirementsMetCondition,
				corev1.ConditionFalse,
				"InstallConfigRefNotSet",
				"Install config reference is not set",
				controllerutils.UpdateConditionIfReasonOrMessageChange)
			if changed {
				cd.Status.Conditions = conditions
				return reconcile.Result{}, r.Status().Update(context.TODO(), cd)
			}
			return reconcile.Result{}, nil
		}

		icSecret := &corev1.Secret{}
		err = r.Get(context.Background(),
			types.NamespacedName{
				Namespace: cd.Namespace,
				Name:      cd.Spec.Provisioning.InstallConfigSecretRef.Name,
			},
			icSecret)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "Error loading install config secret")
			return reconcile.Result{}, err
		}

		_, err = ValidateInstallConfig(cd, icSecret)
		if err != nil {
			cdLog.WithError(err).Info("install config validation failed")
			conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				cd.Status.Conditions,
				hivev1.RequirementsMetCondition,
				corev1.ConditionFalse,
				"InstallConfigValidationFailed",
				err.Error(),
				controllerutils.UpdateConditionIfReasonOrMessageChange)
			if changed {
				cd.Status.Conditions = conditions
				return reconcile.Result{}, r.Status().Update(context.TODO(), cd)
			}
			return reconcile.Result{}, nil
		}

		// If we made it this far, RequirementsMet condition should be True:
		//
		// TODO: when https://github.com/openshift/hive/pull/1413 is implemented
		// we'll want to remove this assumption as ClusterInstall implementations may
		// later indicate their requirements are not met. Instead, we should explicitly clear
		// the condition back to Unknown if we see our Reason set, but the problem is no longer
		// present.
		conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.RequirementsMetCondition,
			corev1.ConditionTrue,
			"AllRequirementsMet",
			"All pre-provision requirements met",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			cd.Status.Conditions = conditions
			err = r.Status().Update(context.TODO(), cd)
			if err != nil {
				return reconcile.Result{}, err
			}
		}

		return r.reconcileInstallingClusterProvision(cd, releaseImage, cdLog)
	case cd.Spec.ClusterInstallRef != nil:
		return r.reconcileInstallingClusterInstall(cd, cdLog)
	default:
		return reconcile.Result{}, errors.New("invalid provisioning configuration")
	}
}

func (r *ReconcileClusterDeployment) reconcileInstallingClusterProvision(cd *hivev1.ClusterDeployment, releaseImage string, logger log.FieldLogger) (reconcile.Result, error) {
	if cd.Status.ProvisionRef == nil {
		return r.startNewProvision(cd, releaseImage, logger)
	}
	return r.reconcileExistingProvision(cd, logger)
}

func (r *ReconcileClusterDeployment) reconcileInstalledClusterProvision(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	// delete failed provisions which are more than 7 days old
	existingProvisions, err := r.existingProvisions(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.deleteOldFailedProvisions(existingProvisions, logger)

	logger.Debug("cluster is already installed, no processing of provision needed")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) reconcileInstallingClusterInstall(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	ref := cd.Spec.ClusterInstallRef
	gvk := schema.GroupVersionKind{
		Group:   ref.Group,
		Version: ref.Version,
		Kind:    ref.Kind,
	}

	if err := r.watchClusterInstall(gvk, logger); err != nil {
		logger.WithField("gvk", gvk.String()).WithError(err).Error("failed to watch for cluster install contract")
		return reconcile.Result{}, err
	}

	return r.reconcileExistingInstallingClusterInstall(cd, logger)
}

func dnsZoneNotReadyMaybeTimedOut(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (string, string, time.Duration) {
	cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.DNSNotReadyCondition)
	if cond == nil || (cond.Reason != dnsNotReadyReason && cond.Reason != dnsNotReadyTimedoutReason) {
		// First time we're setting the condition to "not ready".
		// If the DNSZone becomes ready, it'll pop this controller right away.
		// If it never becomes ready, we want to reconcile again to set the timed-out conditions.
		return dnsNotReadyReason, "DNS Zone not yet available", defaultDNSNotReadyTimeout
	}

	if cond.Reason == dnsNotReadyTimedoutReason {
		// Already timed out previously. Returning the existing values will result in no update and thus no requeue.
		return cond.Reason, cond.Message, 0
	}

	// If we get here, the reason is already dnsNotReadyReason. Decide whether we've timed out.
	timeToTimeout := time.Until(cond.LastTransitionTime.Add(defaultDNSNotReadyTimeout))
	if timeToTimeout <= 0 {
		logger.WithField("timeout", defaultDNSNotReadyTimeout).Warn("Timed out waiting on managed dns creation")
		// We timed out. Set the Provisioned and ProvisionStopped conditions.
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
			cd.Status.Conditions,
			hivev1.ProvisionStoppedCondition,
			corev1.ConditionTrue,
			dnsNotReadyTimedoutReason,
			"Timed out waiting on managed dns creation",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		// This is what shows up under ProvisionStatus in oc tabular output
		cd.Status.Conditions = controllerutils.SetClusterDeploymentCondition(
			cd.Status.Conditions,
			hivev1.ProvisionedCondition,
			corev1.ConditionFalse,
			hivev1.ProvisionedReasonProvisionStopped,
			"Provisioning failed terminally (see the ProvisionStopped condition for details)",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		// The caller will set the DNSNotReady condition per the returns; and, because those are guaranteed to have
		// changed, it'll trigger the Status().Update() that will encompass the other two condition changes above.
		return dnsNotReadyTimedoutReason, "DNS Zone timed out in DNSNotReady state", 0
	}

	// If we get here, the reason is dnsNotReadyReason and we want it to stay that way.
	// If the DNSZone becomes ready, it'll pop this controller right away.
	// If it never becomes ready, we want to reconcile again to set the timed-out conditions.
	return cond.Reason, cond.Message, timeToTimeout
}

// getReleaseImage looks for a a release image in clusterdeployment or its corresponding imageset in the following order:
// 1 - specified in the cluster deployment spec.images.releaseImage
// 2 - referenced in the cluster deployment spec.imageSet
func (r *ReconcileClusterDeployment) getReleaseImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Provisioning != nil && cd.Spec.Provisioning.ReleaseImage != "" {
		return cd.Spec.Provisioning.ReleaseImage
	}
	if imageSet != nil {
		return imageSet.Spec.ReleaseImage
	}
	return ""
}

func (r *ReconcileClusterDeployment) getClusterImageSet(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (*hivev1.ClusterImageSet, error) {
	imageSetKey := types.NamespacedName{}

	switch {
	case cd.Spec.Provisioning != nil:
		imageSetKey.Name = getClusterImageSetFromProvisioning(cd)
		if imageSetKey.Name == "" {
			return nil, nil
		}
	case cd.Spec.ClusterInstallRef != nil:
		isName, err := getClusterImageSetFromClusterInstall(r.Client, cd)
		if err != nil {
			return nil, err
		}
		imageSetKey.Name = isName
	default:
		cdLog.Warning("clusterdeployment references no clusterimageset")
		if err := r.setReqsMetConditionImageSetNotFound(cd, "unknown", true, cdLog); err != nil {
			return nil, err
		}
	}

	imageSet := &hivev1.ClusterImageSet{}
	err := r.Get(context.TODO(), imageSetKey, imageSet)
	if apierrors.IsNotFound(err) {
		cdLog.WithField("clusterimageset", imageSetKey.Name).
			Warning("clusterdeployment references non-existent clusterimageset")
		if err := r.setReqsMetConditionImageSetNotFound(cd, imageSetKey.Name, true, cdLog); err != nil {
			return nil, err
		}
		return nil, err
	}
	if err != nil {
		cdLog.WithError(err).WithField("clusterimageset", imageSetKey.Name).
			Error("unexpected error retrieving clusterimageset")
		return nil, err
	}
	if err := r.setReqsMetConditionImageSetNotFound(cd, imageSetKey.Name, false, cdLog); err != nil {
		return nil, err
	}
	return imageSet, nil
}

func (r *ReconcileClusterDeployment) statusUpdate(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot update clusterdeployment status")
	}
	return err
}

const (
	imagesResolvedReason = "ImagesResolved"
	imagesResolvedMsg    = "Images required for cluster deployment installations are resolved"
)

func (r *ReconcileClusterDeployment) resolveInstallerImage(cd *hivev1.ClusterDeployment, releaseImage string, cdLog log.FieldLogger) (*reconcile.Result, error) {
	areImagesResolved := cd.Status.InstallerImage != nil && cd.Status.CLIImage != nil

	jobKey := client.ObjectKey{Namespace: cd.Namespace, Name: imageset.GetImageSetJobName(cd.Name)}
	jobLog := cdLog.WithField("job", jobKey.Name)

	existingJob := &batchv1.Job{}
	switch err := r.Get(context.Background(), jobKey, existingJob); {
	// The job does not exist. If the images have been resolved, continue reconciling. Otherwise, create the job.
	case apierrors.IsNotFound(err):
		if areImagesResolved {
			return nil, r.updateCondition(cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionFalse, imagesResolvedReason, imagesResolvedMsg, cdLog)
		}

		job := imageset.GenerateImageSetJob(cd, releaseImage, controllerutils.InstallServiceAccountName,
			os.Getenv("HTTP_PROXY"),
			os.Getenv("HTTPS_PROXY"),
			os.Getenv("NO_PROXY"))

		cdLog.WithField("derivedObject", job.Name).Debug("Setting labels on derived object")
		job.Labels = k8slabels.AddLabel(job.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
		job.Labels = k8slabels.AddLabel(job.Labels, constants.JobTypeLabel, constants.JobTypeImageSet)
		if err := controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
			cdLog.WithError(err).Error("error setting controller reference on job")
			return nil, err
		}

		jobLog.WithField("releaseImage", releaseImage).Info("creating imageset job")
		err = controllerutils.SetupClusterInstallServiceAccount(r, cd.Namespace, cdLog)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error setting up service account and role")
			return nil, err
		}

		if err := r.Create(context.TODO(), job); err != nil {
			jobLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating job")
			return nil, err
		}
		// kickstartDuration calculates the delay between creation of cd and start of imageset job
		kickstartDuration := time.Since(cd.CreationTimestamp.Time)
		cdLog.WithField("elapsed", kickstartDuration.Seconds()).Info("calculated time to imageset job seconds")
		metricImageSetDelaySeconds.Observe(float64(kickstartDuration.Seconds()))
		return &reconcile.Result{}, nil

	// There was an error getting the job. Return the error.
	case err != nil:
		jobLog.WithError(err).Error("cannot get job")
		return nil, err

	// The job exists and is in the process of getting deleted. If the images were resolved, then continue reconciling.
	// If the images were not resolved, requeue and wait for the delete to complete.
	case !existingJob.DeletionTimestamp.IsZero():
		if areImagesResolved {
			return nil, r.updateCondition(cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionFalse, imagesResolvedReason, imagesResolvedMsg, cdLog)
		}
		jobLog.Debug("imageset job is being deleted. Will recreate once deleted")
		return &reconcile.Result{RequeueAfter: defaultRequeueTime}, err

	// If job exists and is finished, delete it. If the images were not resolved, then the job will be re-created.
	case controllerutils.IsFinished(existingJob):
		jobLog.WithField("successful", controllerutils.IsSuccessful(existingJob)).
			Warning("Finished job found. Deleting.")
		if err := r.Delete(
			context.Background(),
			existingJob,
			client.PropagationPolicy(metav1.DeletePropagationForeground),
		); err != nil {
			jobLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot delete imageset job")
			return nil, err
		}
		if areImagesResolved {
			return nil, r.updateCondition(cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionFalse, imagesResolvedReason, imagesResolvedMsg, cdLog)
		}

		// the job has failed to update the images and therefore
		// we need to update the InstallImagesResolvedCondition to reflect why it failed.
		for _, jcond := range existingJob.Status.Conditions {
			if jcond.Type != batchv1.JobFailed {
				continue
			}

			msg := fmt.Sprintf("The job %s/%s to resolve the image failed because of (%s) %s",
				existingJob.Namespace, existingJob.Name,
				jcond.Reason, jcond.Message,
			)
			return &reconcile.Result{}, r.updateCondition(cd, hivev1.InstallImagesNotResolvedCondition, corev1.ConditionTrue, "JobToResolveImagesFailed", msg, cdLog)
		}

		return &reconcile.Result{}, nil

	// The job exists and is in progress. Wait for the job to finish before doing any more reconciliation.
	default:
		jobLog.Debug("job exists and is in progress")
		return &reconcile.Result{}, nil
	}
}

func (r *ReconcileClusterDeployment) updateCondition(
	cd *hivev1.ClusterDeployment,
	ctype hivev1.ClusterDeploymentConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	cdLog log.FieldLogger) error {
	conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		ctype,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	if !changed {
		return nil
	}
	cd.Status.Conditions = conditions
	cdLog.Debugf("setting %s Condition to %v", ctype, status)
	return r.Status().Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) setAuthenticationFailure(cd *hivev1.ClusterDeployment, authSuccessful bool, authError error, cdLog log.FieldLogger) (bool, error) {

	var status corev1.ConditionStatus
	var reason, message string

	if authSuccessful {
		status = corev1.ConditionFalse
		reason = platformAuthSuccessReason
		message = "Platform credentials passed authentication check"
	} else {
		status = corev1.ConditionTrue
		reason = platformAuthFailureReason
		message = fmt.Sprintf("Platform credentials failed authentication check: %s", authError)
	}

	conditions, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.AuthenticationFailureClusterDeploymentCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	if !changed {
		return false, nil
	}
	cd.Status.Conditions = conditions

	return changed, r.Status().Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) setReqsMetConditionImageSetNotFound(cd *hivev1.ClusterDeployment, name string, isNotFound bool, cdLog log.FieldLogger) error {
	var changed bool
	var conds []hivev1.ClusterDeploymentCondition

	if isNotFound {
		conds, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.RequirementsMetCondition,
			corev1.ConditionFalse,
			clusterImageSetNotFoundReason,
			fmt.Sprintf("ClusterImageSet %s is not available", name),
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	} else {
		// Set the RequirementsMet condition status back to unknown if True and it's current reason matches
		reqsCond := controllerutils.FindCondition(cd.Status.Conditions,
			hivev1.RequirementsMetCondition)
		if reqsCond.Status == corev1.ConditionFalse && reqsCond.Reason == clusterImageSetNotFoundReason {
			conds, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				cd.Status.Conditions,
				hivev1.RequirementsMetCondition,
				corev1.ConditionUnknown,
				clusterImageSetFoundReason,
				fmt.Sprintf("ClusterImageSet %s is available", name),
				controllerutils.UpdateConditionIfReasonOrMessageChange)
		}
	}
	if !changed {
		return nil
	}
	cd.Status.Conditions = conds
	reqsCond := controllerutils.FindCondition(cd.Status.Conditions,
		hivev1.RequirementsMetCondition)
	cdLog.Infof("updating RequirementsMetCondition: status=%s reason=%s", reqsCond.Status, reqsCond.Reason)
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot update status conditions")
	}
	return err
}

// setClusterStatusURLs fetches the openshift console route from the remote cluster and uses it to determine
// the correct APIURL and WebConsoleURL, and then set them in the Status. Typically only called if these Status fields
// are unset.
func (r *ReconcileClusterDeployment) setClusterStatusURLs(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	server, err := remoteclient.InitialURL(r.Client, cd)
	if err != nil {
		cdLog.WithError(err).Error("could not get API URL from kubeconfig")
		return reconcile.Result{}, err
	}
	cdLog.Debugf("found cluster API URL in kubeconfig: %s", server)
	cd.Status.APIURL = server

	remoteClient, unreachable, requeue := remoteclient.ConnectToRemoteCluster(
		cd,
		r.remoteClusterAPIClientBuilder(cd),
		r.Client,
		cdLog,
	)
	if unreachable {
		return reconcile.Result{Requeue: requeue}, nil
	}

	routeObject := &routev1.Route{}
	if err := remoteClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "openshift-console", Name: "console"},
		routeObject,
	); err != nil {
		cdLog.WithError(err).Info("error fetching remote route object")
		return reconcile.Result{Requeue: true}, nil
	}
	cdLog.Debugf("read remote route object: %s", routeObject)
	cd.Status.WebConsoleURL = "https://" + routeObject.Spec.Host

	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not set cluster status URLs")
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

// ensureManagedDNSZoneDeleted is a safety check to ensure that the child managed DNSZone
// linked to the parent cluster deployment gets a deletionTimestamp when the parent is deleted.
// Normally we expect Kube garbage collection to do this for us, but in rare cases we've seen it
// not working as intended.
func (r *ReconcileClusterDeployment) ensureManagedDNSZoneDeleted(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (gone bool, returnErr error) {
	if !cd.Spec.ManageDNS {
		return true, nil
	}
	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
	switch err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone); {
	case apierrors.IsNotFound(err):
		cdLog.Debug("dnszone has been removed from storage")
		return true, nil
	case err != nil:
		cdLog.WithError(err).Error("error looking up managed dnszone")
		return false, err
	case !dnsZone.DeletionTimestamp.IsZero():
		cdLog.Debug("dnszone has been deleted but is still in storage")
		return false, nil
	}
	if err := r.Delete(context.TODO(), dnsZone); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting managed dnszone")
		return false, err
	}
	return false, nil
}

// namespaceTerminated checks whether the namespace named `nsName` is marked for deletion.
// Note that we are *not* checking whether the namespace has been deleted. The caller has
// the option to check IsNotFound(err) if that is needed.
func (r *ReconcileClusterDeployment) namespaceTerminated(nsName string) (bool, error) {
	ns := &corev1.Namespace{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: nsName}, ns); err != nil {
		// You may think we should be checking IsNotFound here and reporting true; but that's
		// not what the caller is expecting.
		return false, err
	}
	return ns.DeletionTimestamp != nil, nil
}

func (r *ReconcileClusterDeployment) ensureClusterDeprovisioned(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (deprovisioned bool, returnErr error) {
	// Skips/terminates deprovision if PreserveOnDelete is true. If there is ongoing deprovision we abandon it
	if cd.Spec.PreserveOnDelete {
		cdLog.Warn("skipping/terminating deprovision for cluster due to PreserveOnDelete=true, removing finalizer")
		return true, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Warn("skipping uninstall for cluster that never had clusterID set")
		return true, nil
	}

	// We do not yet support deprovision for BareMetal, for now skip deprovision and remove finalizer.
	if cd.Spec.Platform.BareMetal != nil {
		cdLog.Info("skipping deprovision for BareMetal cluster, removing finalizer")
		return true, nil
	}

	if cd.Spec.ClusterInstallRef != nil {
		cdLog.Info("skipping deprovision as it should be done by deleting the obj in cluster install reference")
		return true, nil
	}

	// Generate a deprovision request
	request, err := generateDeprovision(cd)
	if err != nil {
		cdLog.WithError(err).Error("error generating deprovision request")
		return false, err
	}

	cdLog.WithField("derivedObject", request.Name).Debug("Setting label on derived object")
	request.Labels = k8slabels.AddLabel(request.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
	err = controllerutil.SetControllerReference(cd, request, r.scheme)
	if err != nil {
		cdLog.Errorf("error setting controller reference on deprovision request: %v", err)
		return false, err
	}

	// Check if deprovision request already exists:
	existingRequest := &hivev1.ClusterDeprovision{}
	switch err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Name, Namespace: cd.Namespace}, existingRequest); {
	case apierrors.IsNotFound(err):
		cdLog.Info("creating deprovision request for cluster deployment")
		switch err = r.Create(context.TODO(), request); {
		case apierrors.IsAlreadyExists(err):
			cdLog.Info("deprovision request already exists")
			return false, nil
		case err != nil:
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating deprovision request")
			// Check if namespace is terminated, if so we can give up, remove the finalizer, and let
			// the cluster go away.
			deleted, err := r.namespaceTerminated(cd.Namespace)
			if err != nil {
				cdLog.WithError(err).Error("error checking for deletionTimestamp on namespace")
				return false, err
			}
			if deleted {
				cdLog.Warn("detected a namespace deleted before deprovision request could be created, giving up on deprovision and removing finalizer")
				return true, err
			}
			return false, err
		default:
			// Successfully created the ClusterDeprovision. Update the Provisioned CD status condition accordingly.
			return false, r.updateCondition(cd,
				hivev1.ProvisionedCondition,
				corev1.ConditionFalse,
				hivev1.ProvisionedReasonDeprovisioning,
				"Cluster is being deprovisioned",
				cdLog)
		}
	case err != nil:
		cdLog.WithError(err).Error("error getting deprovision request")
		return false, err
	}

	authenticationFailureCondition := controllerutils.FindCondition(existingRequest.Status.Conditions, hivev1.AuthenticationFailureClusterDeprovisionCondition)
	if authenticationFailureCondition != nil {
		var conds []hivev1.ClusterDeploymentCondition
		var changed1, changed2, authFailure bool
		conds, changed1 = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.DeprovisionLaunchErrorCondition,
			authenticationFailureCondition.Status,
			authenticationFailureCondition.Reason,
			authenticationFailureCondition.Message,
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if authenticationFailureCondition.Status == corev1.ConditionTrue {
			authFailure = true
			conds, changed2 = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				conds,
				hivev1.ProvisionedCondition,
				corev1.ConditionFalse,
				hivev1.ProvisionedReasonDeprovisionFailed,
				"Cluster deprovision failed",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
		}
		if changed1 || changed2 {
			cd.Status.Conditions = conds
			return false, r.Status().Update(context.TODO(), cd)
		}
		if authFailure {
			// We get here if we had already set Provisioned to DeprovisionFailed
			return false, nil
		}
	}

	if !existingRequest.Status.Completed {
		cdLog.Debug("deprovision request not yet completed")
		return false, r.updateCondition(
			cd,
			hivev1.ProvisionedCondition,
			corev1.ConditionFalse,
			hivev1.ProvisionedReasonDeprovisioning,
			"Cluster is deprovisioning",
			cdLog,
		)
	}

	// Deprovision succeeded
	return true, r.updateCondition(
		cd,
		hivev1.ProvisionedCondition,
		corev1.ConditionFalse,
		hivev1.ProvisionedReasonDeprovisioned,
		"Cluster is deprovisioned",
		cdLog,
	)
}

func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	switch _, relocateStatus, err := controllerutils.IsRelocating(cd); {
	case err != nil:
		cdLog.WithError(err).Error("could not determine relocate status")
		return reconcile.Result{}, errors.Wrap(err, "could not determine relocate status")
	case relocateStatus == hivev1.RelocateComplete:
		cdLog.Info("clusterdeployment relocated, removing finalizer")
		err := r.removeClusterDeploymentFinalizer(cd, cdLog)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
		}
		return reconcile.Result{}, err
	case relocateStatus != "":
		cdLog.Debug("ClusterDeployment is in the middle of a relocate. Wait until relocate has been completed or aborted before doing finalization.")
		return reconcile.Result{}, nil
	}

	if controllerutils.IsDeleteProtected(cd) {
		cdLog.Error("deprovision blocked for ClusterDeployment with protected delete on")
		return reconcile.Result{}, nil
	}

	dnsZoneGone, err := r.ensureManagedDNSZoneDeleted(cd, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Wait for outstanding provision to be removed before creating deprovision request
	switch result, err := r.stopProvisioning(cd, cdLog); {
	case result != nil:
		return *result, err
	case err != nil:
		return reconcile.Result{}, err
	}

	deprovisioned, err := r.ensureClusterDeprovisioned(cd, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	if deprovisioned {
		if err := r.releaseCustomization(cd, cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error releasing inventory customization")
			return reconcile.Result{}, err
		}
	}

	switch {
	case !deprovisioned:
		return reconcile.Result{}, nil
	case !dnsZoneGone:
		return reconcile.Result{RequeueAfter: defaultRequeueTime}, nil
	default:
		cdLog.Infof("DNSZone gone, customization gone and deprovision request completed, removing deprovision finalizer")
		if err := r.removeClusterDeploymentFinalizer(cd, cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error removing finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
}

func (r *ReconcileClusterDeployment) addClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	controllerutils.AddFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) removeClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {

	cd = cd.DeepCopy()
	controllerutils.DeleteFinalizer(cd, hivev1.FinalizerDeprovision)
	if err := r.Update(context.TODO(), cd); err != nil {
		return err
	}

	// Increment the clusters deleted counter:
	metricClustersDeleted.Observe(cd, nil, 1)
	return nil
}

func (r *ReconcileClusterDeployment) releaseCustomization(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	cpRef := cd.Spec.ClusterPoolRef
	if cpRef == nil || cpRef.CustomizationRef == nil {
		return nil
	}

	cdc := &hivev1.ClusterDeploymentCustomization{}
	cdcNamespace := cpRef.Namespace
	cdcName := cpRef.CustomizationRef.Name
	cdcLog := cdLog.WithField("customization", cdcName).WithField("namespace", cdcNamespace)
	err := r.Client.Get(context.TODO(), client.ObjectKey{Namespace: cdcNamespace, Name: cdcName}, cdc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cdcLog.Info("customization not found, nothing to release")
			return nil
		}
		cdcLog.WithError(err).Error("error reading customization")
		return err
	}

	changed := conditionsv1.SetStatusConditionNoHeartbeat(&cdc.Status.Conditions, conditionsv1.Condition{
		Type:    conditionsv1.ConditionAvailable,
		Status:  corev1.ConditionTrue,
		Reason:  "Available",
		Message: "available",
	})

	if cdc.Status.ClusterPoolRef != nil || cdc.Status.ClusterDeploymentRef != nil {
		cdc.Status.ClusterPoolRef = nil
		cdc.Status.ClusterDeploymentRef = nil
		changed = true
	}

	if changed {
		if err := r.Status().Update(context.Background(), cdc); err != nil {
			cdcLog.WithError(err).Error("failed to update ClusterDeploymentCustomizationAvailable condition")
			return err
		}
	}

	return nil
}

// setDNSDelayMetric will calculate the amount of time elapsed from clusterdeployment creation
// to when the dnszone became ready, and set a metric to report the delay.
// Will return a bool indicating whether the clusterdeployment has been modified, and whether any error was encountered.
func (r *ReconcileClusterDeployment) setDNSDelayMetric(cd *hivev1.ClusterDeployment, dnsZone *hivev1.DNSZone, cdLog log.FieldLogger) (bool, error) {
	initializeAnnotations(cd)

	if _, ok := cd.Annotations[dnsReadyAnnotation]; ok {
		// already have recorded the dnsdelay metric
		return false, nil
	}

	readyCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)

	if readyCondition == nil || readyCondition.Status != corev1.ConditionTrue {
		msg := "did not find timestamp for when dnszone became ready"
		cdLog.WithField("dnszone", dnsZone.Name).Error(msg)
		return false, fmt.Errorf(msg)
	}

	dnsDelayDuration := readyCondition.LastTransitionTime.Sub(cd.CreationTimestamp.Time)
	cdLog.WithField("duration", dnsDelayDuration.Seconds()).Info("DNS ready")
	cd.Annotations[dnsReadyAnnotation] = dnsDelayDuration.String()
	if err := r.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to save annotation marking DNS becoming ready")
		return false, err
	}

	metricDNSDelaySeconds.Observe(float64(dnsDelayDuration.Seconds()))

	return true, nil
}

// ensureDNSZonePreserveOnDeleteAndLogAnnotations makes sure the DNSZone, if one exists, has a
// matching PreserveOnDelete setting and additional log annotations with its ClusterDeployment.
// Returns true if a change was made.
func (r *ReconcileClusterDeployment) ensureDNSZonePreserveOnDeleteAndLogAnnotations(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, error) {
	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
	logger := cdLog.WithField("zone", dnsZoneNamespacedName.String())
	err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone)
	if err != nil && !apierrors.IsNotFound(err) {
		logger.WithError(err).Error("failed to fetch DNS zone")
		return false, err
	} else if err != nil {
		// DNSZone doesn't exist yet
		return false, nil
	}

	changed := controllerutils.CopyLogAnnotation(cd, dnsZone)

	if dnsZone.Spec.PreserveOnDelete != cd.Spec.PreserveOnDelete {
		changed = true
		logger.WithField("preserveOnDelete", cd.Spec.PreserveOnDelete).Info("setting DNSZone PreserveOnDelete to match ClusterDeployment PreserveOnDelete")
		dnsZone.Spec.PreserveOnDelete = cd.Spec.PreserveOnDelete
	}

	if changed {
		err := r.Update(context.TODO(), dnsZone)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error updating DNSZone")
			return false, err
		}
	}

	return changed, nil
}

// ensureManagedDNSZone
// - Makes sure a DNSZone object exists for this CD, creating it if it does not already exist.
// - Parlays the DNSZone's status conditions into the CD's DNSZoneNotReady condition.
// - Observes metricDNSDelaySeconds.
// Returns:
//   - bool: true if the caller should return from the reconcile loop, using the...
//   - Result: suitable for returning from the reconcile loop. If the DNSZone is not ready, it will include a delay to retrigger once
//     we've waited long enough for it. If the DNSZone becomes ready in the meantime, its update will trigger this controller earlier.
//   - error: the usual; if not nil, the caller should requeue
func (r *ReconcileClusterDeployment) ensureManagedDNSZone(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, reconcile.Result, error) {
	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
	case p.GCP != nil:
	case p.Azure != nil:
	default:
		cdLog.Error("cluster deployment platform does not support managed DNS")
		if err := r.updateCondition(cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue, dnsUnsupportedPlatformReason, "Managed DNS is not supported on specified platform", cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition for DNSUnsupportedPlatform reason")
			return false, reconcile.Result{}, err
		}
		// TODO: This really shouldn't requeue; it would hot loop the controller.
		return false, reconcile.Result{}, errors.New("managed DNS not supported on platform")
	}

	dnsZone := &hivev1.DNSZone{}
	dnsZoneNamespacedName := types.NamespacedName{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}
	logger := cdLog.WithField("zone", dnsZoneNamespacedName.String())

	switch err := r.Get(context.TODO(), dnsZoneNamespacedName, dnsZone); {
	case apierrors.IsNotFound(err):
		logger.Info("creating new DNSZone for cluster deployment")
		return true, reconcile.Result{}, r.createManagedDNSZone(cd, logger)
	case err != nil:
		logger.WithError(err).Error("failed to fetch DNS zone")
		return false, reconcile.Result{}, err
	}

	if !metav1.IsControlledBy(dnsZone, cd) {
		cdLog.Error("DNS zone already exists but is not owned by cluster deployment")
		if err := r.updateCondition(cd, hivev1.DNSNotReadyCondition, corev1.ConditionTrue, dnsZoneResourceConflictReason, "Existing DNS zone not owned by cluster deployment", cdLog); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
			return false, reconcile.Result{}, err
		}
		// TODO: This really shouldn't requeue; it would hot loop the controller.
		return true, reconcile.Result{}, errors.New("Existing unowned DNS zone")
	}

	availableCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
	insufficientCredentialsCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.InsufficientCredentialsCondition)
	authenticationFailureCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.AuthenticationFailureCondition)
	apiOptInRequiredCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.APIOptInRequiredCondition)
	dnsErrorCondition := controllerutils.FindCondition(dnsZone.Status.Conditions, hivev1.GenericDNSErrorsCondition)
	// Cache ProvisionStoppedCondition status, so later we can decide if ProvisionFailedTerminal metric needs to be observed.
	provisionStoppedConditionStatus := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition).Status
	var (
		status                 corev1.ConditionStatus
		reason, message        string
		dnsCondChanged         bool
		stoppedCondChanged     bool
		provisionedCondChanged bool
		metricAnnotationSet    bool
		requeueAfter           time.Duration
		fatalCondition         bool
	)
	switch {
	case availableCondition != nil && availableCondition.Status == corev1.ConditionTrue:
		status = corev1.ConditionFalse
		reason = dnsReadyReason
		message = "DNS Zone available"
	case insufficientCredentialsCondition != nil && insufficientCredentialsCondition.Status == corev1.ConditionTrue:
		status = corev1.ConditionTrue
		reason = "InsufficientCredentials"
		message = insufficientCredentialsCondition.Message
		fatalCondition = true
	case authenticationFailureCondition != nil && authenticationFailureCondition.Status == corev1.ConditionTrue:
		status = corev1.ConditionTrue
		reason = "AuthenticationFailure"
		message = authenticationFailureCondition.Message
		fatalCondition = true
	case apiOptInRequiredCondition != nil && apiOptInRequiredCondition.Status == corev1.ConditionTrue:
		status = corev1.ConditionTrue
		reason = "APIOptInRequiredForDNS"
		message = apiOptInRequiredCondition.Message
		fatalCondition = true
	case dnsErrorCondition != nil && dnsErrorCondition.Status == corev1.ConditionTrue:
		status = corev1.ConditionTrue
		reason = dnsErrorCondition.Reason
		message = dnsErrorCondition.Message
	default:
		status = corev1.ConditionTrue
		reason, message, requeueAfter = dnsZoneNotReadyMaybeTimedOut(cd, cdLog)
		// dnsZoneNotReadyMaybeTimedOut will stop provisioning on its own if it feels the need to
	}
	cd.Status.Conditions, dnsCondChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.DNSNotReadyCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange)

	if fatalCondition {
		logger.WithField("reason", reason).WithField("message", message).Error("DNSZone is in fatal state")
		// Fatal state. Set the Provisioned and ProvisionStopped conditions.
		cd.Status.Conditions, stoppedCondChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.ProvisionStoppedCondition,
			corev1.ConditionTrue,
			"DNSZoneFailedToReconcile",
			"DNSZone failed to reconcile (see the DNSNotReadyCondition condition for details)",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		// This is what shows up under ProvisionStatus in oc tabular output
		cd.Status.Conditions, provisionedCondChanged = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.ProvisionedCondition,
			corev1.ConditionFalse,
			hivev1.ProvisionedReasonProvisionStopped,
			"Provisioning failed terminally (see the ProvisionStopped condition for details)",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
	}

	if dnsCondChanged || stoppedCondChanged || provisionedCondChanged {
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not update DNSNotReadyCondition")
			return false, reconcile.Result{}, err
		}
	}

	// Observe ProvisionFailedTerminal metric if we have set ProvisionStopped.
	if controllerutils.FindCondition(cd.Status.Conditions, hivev1.ProvisionStoppedCondition).Status == corev1.ConditionTrue &&
		provisionStoppedConditionStatus != corev1.ConditionTrue {
		incProvisionFailedTerminal(cd)
	}

	// Only attempt to record the delay metric if the DNSZone is ready
	if status == corev1.ConditionFalse {
		var err error
		metricAnnotationSet, err = r.setDNSDelayMetric(cd, dnsZone, cdLog)
		if err != nil {
			return dnsCondChanged, reconcile.Result{}, err
		}
	}

	requeue := dnsCondChanged || metricAnnotationSet || requeueAfter > 0
	return requeue, reconcile.Result{Requeue: requeue, RequeueAfter: requeueAfter}, nil
}

func (r *ReconcileClusterDeployment) createManagedDNSZone(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	dnsZone := &hivev1.DNSZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      controllerutils.DNSZoneName(cd.Name),
			Namespace: cd.Namespace,
		},
		Spec: hivev1.DNSZoneSpec{
			Zone:               cd.Spec.BaseDomain,
			PreserveOnDelete:   cd.Spec.PreserveOnDelete,
			LinkToParentDomain: true,
		},
	}
	controllerutils.CopyLogAnnotation(cd, dnsZone)

	switch {
	case cd.Spec.Platform.AWS != nil:
		additionalTags := make([]hivev1.AWSResourceTag, 0, len(cd.Spec.Platform.AWS.UserTags))
		for k, v := range cd.Spec.Platform.AWS.UserTags {
			additionalTags = append(additionalTags, hivev1.AWSResourceTag{Key: k, Value: v})
		}
		region := ""
		if strings.HasPrefix(cd.Spec.Platform.AWS.Region, constants.AWSChinaRegionPrefix) {
			region = constants.AWSChinaRoute53Region
		}
		dnsZone.Spec.AWS = &hivev1.AWSDNSZoneSpec{
			CredentialsSecretRef:  cd.Spec.Platform.AWS.CredentialsSecretRef,
			CredentialsAssumeRole: cd.Spec.Platform.AWS.CredentialsAssumeRole,
			AdditionalTags:        additionalTags,
			Region:                region,
		}
	case cd.Spec.Platform.GCP != nil:
		dnsZone.Spec.GCP = &hivev1.GCPDNSZoneSpec{
			CredentialsSecretRef: cd.Spec.Platform.GCP.CredentialsSecretRef,
		}
	case cd.Spec.Platform.Azure != nil:
		dnsZone.Spec.Azure = &hivev1.AzureDNSZoneSpec{
			CredentialsSecretRef: cd.Spec.Platform.Azure.CredentialsSecretRef,
			ResourceGroupName:    cd.Spec.Platform.Azure.BaseDomainResourceGroupName,
			CloudName:            cd.Spec.Platform.Azure.CloudName,
		}
	}

	logger.WithField("derivedObject", dnsZone.Name).Debug("Setting labels on derived object")
	dnsZone.Labels = k8slabels.AddLabel(dnsZone.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
	dnsZone.Labels = k8slabels.AddLabel(dnsZone.Labels, constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild)
	if err := controllerutil.SetControllerReference(cd, dnsZone, r.scheme); err != nil {
		logger.WithError(err).Error("error setting controller reference on dnszone")
		return err
	}

	err := r.Create(context.TODO(), dnsZone)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "cannot create DNS zone")
		return err
	}
	logger.Info("dns zone created")
	return nil
}

func selectorPodWatchHandler(ctx context.Context, a client.Object) []reconcile.Request {
	retval := []reconcile.Request{}

	pod := a.(*corev1.Pod)
	if pod == nil {
		// Wasn't a Pod, bail out. This should not happen.
		log.Errorf("Error converting MapObject.Object to Pod. Value: %+v", a)
		return retval
	}
	if pod.Labels == nil {
		return retval
	}
	cdName, ok := pod.Labels[constants.ClusterDeploymentNameLabel]
	if !ok {
		return retval
	}
	retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      cdName,
		Namespace: pod.Namespace,
	}})
	return retval
}

func generateDeprovision(cd *hivev1.ClusterDeployment) (*hivev1.ClusterDeprovision, error) {
	req := &hivev1.ClusterDeprovision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cd.Name,
			Namespace: cd.Namespace,
		},
		Spec: hivev1.ClusterDeprovisionSpec{
			InfraID:     cd.Spec.ClusterMetadata.InfraID,
			ClusterID:   cd.Spec.ClusterMetadata.ClusterID,
			ClusterName: cd.Spec.ClusterName,
		},
	}
	controllerutils.CopyLogAnnotation(cd, req)

	switch {
	case cd.Spec.Platform.AWS != nil:
		req.Spec.Platform.AWS = &hivev1.AWSClusterDeprovision{
			Region:                cd.Spec.Platform.AWS.Region,
			CredentialsSecretRef:  &cd.Spec.Platform.AWS.CredentialsSecretRef,
			CredentialsAssumeRole: cd.Spec.Platform.AWS.CredentialsAssumeRole,
		}
	case cd.Spec.Platform.Azure != nil:
		// If we haven't discovered the ResourceGroupName yet, fail
		rg, err := controllerutils.AzureResourceGroup(cd)
		if err != nil {
			return nil, err
		}
		req.Spec.Platform.Azure = &hivev1.AzureClusterDeprovision{
			CredentialsSecretRef: &cd.Spec.Platform.Azure.CredentialsSecretRef,
			CloudName:            &cd.Spec.Platform.Azure.CloudName,
			ResourceGroupName:    &rg,
		}
	case cd.Spec.Platform.GCP != nil:
		req.Spec.Platform.GCP = &hivev1.GCPClusterDeprovision{
			Region:               cd.Spec.Platform.GCP.Region,
			CredentialsSecretRef: &cd.Spec.Platform.GCP.CredentialsSecretRef,
		}
	case cd.Spec.Platform.OpenStack != nil:
		req.Spec.Platform.OpenStack = &hivev1.OpenStackClusterDeprovision{
			Cloud:                 cd.Spec.Platform.OpenStack.Cloud,
			CredentialsSecretRef:  &cd.Spec.Platform.OpenStack.CredentialsSecretRef,
			CertificatesSecretRef: cd.Spec.Platform.OpenStack.CertificatesSecretRef,
		}
	case cd.Spec.Platform.VSphere != nil:
		req.Spec.Platform.VSphere = &hivev1.VSphereClusterDeprovision{
			CredentialsSecretRef:  cd.Spec.Platform.VSphere.CredentialsSecretRef,
			CertificatesSecretRef: cd.Spec.Platform.VSphere.CertificatesSecretRef,
			VCenter:               cd.Spec.Platform.VSphere.VCenter,
		}
	case cd.Spec.Platform.Ovirt != nil:
		req.Spec.Platform.Ovirt = &hivev1.OvirtClusterDeprovision{
			CredentialsSecretRef:  cd.Spec.Platform.Ovirt.CredentialsSecretRef,
			CertificatesSecretRef: cd.Spec.Platform.Ovirt.CertificatesSecretRef,
			ClusterID:             cd.Spec.Platform.Ovirt.ClusterID,
		}
	case cd.Spec.Platform.IBMCloud != nil:
		req.Spec.Platform.IBMCloud = &hivev1.IBMClusterDeprovision{
			CredentialsSecretRef: cd.Spec.Platform.IBMCloud.CredentialsSecretRef,
			Region:               cd.Spec.Platform.IBMCloud.Region,
			BaseDomain:           cd.Spec.BaseDomain,
		}
	case cd.Spec.Platform.AlibabaCloud != nil:
		req.Spec.Platform.AlibabaCloud = &hivev1.AlibabaCloudClusterDeprovision{
			CredentialsSecretRef: cd.Spec.Platform.AlibabaCloud.CredentialsSecretRef,
			Region:               cd.Spec.Platform.AlibabaCloud.Region,
			BaseDomain:           cd.Spec.BaseDomain,
		}
	default:
		return nil, errors.New("unsupported cloud provider for deprovision")
	}

	return req, nil
}

func generatePullSecretObj(pullSecret string, pullSecretName string, cd *hivev1.ClusterDeployment) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: cd.Namespace,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		StringData: map[string]string{
			corev1.DockerConfigJsonKey: pullSecret,
		},
	}
}

// initializeAnnotations() initializes the annotations if it is not already
func initializeAnnotations(cd *hivev1.ClusterDeployment) {
	if cd.Annotations == nil {
		cd.Annotations = map[string]string{}
	}
}

// mergePullSecrets merges the global pull secret JSON (if defined) with the cluster's pull secret JSON (if defined)
// An error will be returned if neither is defined
func (r *ReconcileClusterDeployment) mergePullSecrets(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (string, error) {
	var localPullSecret string
	var err error

	// For code readability let's call the pull secret in cluster deployment config as local pull secret
	if cd.Spec.PullSecretRef != nil {
		localPullSecret, err = controllerutils.LoadSecretData(r.Client, cd.Spec.PullSecretRef.Name, cd.Namespace, corev1.DockerConfigJsonKey)
		if err != nil {
			return "", errors.Wrap(err, "local pull secret could not be retrieved")
		}
	}

	// Check if global pull secret from env as it comes from hive config
	globalPullSecretName := os.Getenv(constants.GlobalPullSecret)
	var globalPullSecret string
	if len(globalPullSecretName) != 0 {
		globalPullSecret, err = controllerutils.LoadSecretData(r.Client, globalPullSecretName, controllerutils.GetHiveNamespace(), corev1.DockerConfigJsonKey)
		if err != nil {
			return "", errors.Wrap(err, "global pull secret could not be retrieved")
		}
	}

	switch {
	case globalPullSecret != "" && localPullSecret != "":
		// Merge local pullSecret and globalPullSecret. If both pull secrets have same registry name
		// then the merged pull secret will have registry secret from local pull secret
		pullSecret, err := controllerutils.MergeJsons(globalPullSecret, localPullSecret, cdLog)
		if err != nil {
			errMsg := "unable to merge global pull secret with local pull secret"
			cdLog.WithError(err).Error(errMsg)
			return "", errors.Wrap(err, errMsg)
		}
		return pullSecret, nil
	case globalPullSecret != "":
		return globalPullSecret, nil
	case localPullSecret != "":
		return localPullSecret, nil
	default:
		errMsg := "clusterdeployment must specify pull secret since hiveconfig does not specify a global pull secret"
		cdLog.Error(errMsg)
		return "", errors.New(errMsg)
	}
}

// updatePullSecretInfo creates or updates the merged pull secret for the clusterdeployment.
// It returns true when the merged pull secret has been created or updated.
func (r *ReconcileClusterDeployment) updatePullSecretInfo(pullSecret string, cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (bool, error) {
	var err error
	pullSecretObjExists := true
	existingPullSecretObj := &corev1.Secret{}
	mergedSecretName := constants.GetMergedPullSecretName(cd)
	err = r.Get(context.TODO(), types.NamespacedName{Name: mergedSecretName, Namespace: cd.Namespace}, existingPullSecretObj)
	if err != nil {
		if apierrors.IsNotFound(err) {
			cdLog.Info("Existing pull secret object not found")
			pullSecretObjExists = false
		} else {
			return false, errors.Wrap(err, "Error getting pull secret from cluster deployment")
		}
	}

	if pullSecretObjExists {
		existingPullSecret, ok := existingPullSecretObj.Data[corev1.DockerConfigJsonKey]
		if !ok {
			return false, fmt.Errorf("pull secret %s did not contain key %s", mergedSecretName, corev1.DockerConfigJsonKey)
		}
		if string(existingPullSecret) == pullSecret {
			cdLog.Debug("Existing and the new merged pull secret are same")
			return false, nil
		}
		cdLog.Info("Existing merged pull secret hash did not match with latest merged pull secret")
		existingPullSecretObj.Data[corev1.DockerConfigJsonKey] = []byte(pullSecret)
		err = r.Update(context.TODO(), existingPullSecretObj)
		if err != nil {
			return false, errors.Wrap(err, "error updating merged pull secret object")
		}
		cdLog.WithField("secretName", mergedSecretName).Info("Updated the merged pull secret object successfully")
	} else {

		// create a new pull secret object
		newPullSecretObj := generatePullSecretObj(
			pullSecret,
			mergedSecretName,
			cd,
		)

		cdLog.WithField("derivedObject", newPullSecretObj.Name).Debug("Setting labels on derived object")
		newPullSecretObj.Labels = k8slabels.AddLabel(newPullSecretObj.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
		newPullSecretObj.Labels = k8slabels.AddLabel(newPullSecretObj.Labels, constants.SecretTypeLabel, constants.SecretTypeMergedPullSecret)
		err = controllerutil.SetControllerReference(cd, newPullSecretObj, r.scheme)
		if err != nil {
			cdLog.Errorf("error setting controller reference on new merged pull secret: %v", err)
			return false, err
		}
		err = r.Create(context.TODO(), newPullSecretObj)
		if err != nil {
			return false, errors.Wrap(err, "error creating new pull secret object")
		}
		cdLog.WithField("secretName", mergedSecretName).Info("Created the merged pull secret object successfully")
	}
	return true, nil
}

func configureTrustedCABundleConfigMap(cm *corev1.ConfigMap, cd *hivev1.ClusterDeployment) bool {
	modified := false
	if cm.Labels == nil {
		cm.Labels = make(map[string]string)
	}
	if cm.Labels[injectCABundleKey] != "true" {
		cm.Labels[injectCABundleKey] = "true"
		modified = true
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	if _, ok := cm.Data[constants.TrustedCABundleFile]; !ok {
		cm.Data[constants.TrustedCABundleFile] = ""
		modified = true
	}

	return modified
}

// ensureTrustedCABundleConfigMap makes sure there is a ConfigMap in the cd's namespace with the magical
// config.opesnhift.io/inject-trusted-cabundle="true" label, which causes the Cluster Network Operator to
// populate it with a merged CA bundle including the cluster proxy's trustedCA if configured. We'll mount
// this ConfigMap on all prov/deprov pods. See https://docs.openshift.com/container-platform/4.12/networking/configuring-a-custom-pki.html#certificate-injection-using-operators_configuring-a-custom-pki
func (r *ReconcileClusterDeployment) ensureTrustedCABundleConfigMap(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	cm := &corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: constants.TrustedCAConfigMapName}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			cm.Name = constants.TrustedCAConfigMapName
			cm.Namespace = cd.Namespace
			if cm.Labels == nil {
				cm.Labels = make(map[string]string)
			}
			configureTrustedCABundleConfigMap(cm, cd)
			if err := r.Create(context.TODO(), cm); err != nil {
				deleted, err2 := r.namespaceTerminated(cd.Namespace)
				if deleted {
					cdLog.Warn("detected deleted namespace; skipping creation of CA bundle ConfigMap")
					// Pretend this is not an error
					return nil
				}
				msg := "Failed to create the trusted CA bundle ConfigMap"
				if err2 != nil {
					cdLog.WithError(err).Warn(msg)
					return errors.Wrapf(err2, "Failed to discover whether namespace %s is marked for deletion", cd.Namespace)
				}
				return errors.Wrap(err, msg)
			}
		} else {
			return errors.Wrap(err, "Failed to retrieve trusted CA bundle ConfigMap")
		}
	}
	if configureTrustedCABundleConfigMap(cm, cd) {
		if err := r.Update(context.TODO(), cm); err != nil {
			return errors.Wrap(err, "Failed to update the trusted CA bundle ConfigMap")
		}
	}
	return nil
}

func calculateNextProvisionTime(failureTime time.Time, retries int, cdLog log.FieldLogger) time.Time {
	// (2^currentRetries) * 60 seconds up to a max of 24 hours.
	const sleepCap = 24 * time.Hour
	const retryCap = 11 // log_2_(24*60)

	if retries >= retryCap {
		return failureTime.Add(sleepCap)
	}
	return failureTime.Add((1 << uint(retries)) * time.Minute)
}

// existingProvisions returns the list of ClusterProvisions associated with the specified
// ClusterDeployment, sorted by age, oldest first.
func (r *ReconcileClusterDeployment) existingProvisions(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) ([]*hivev1.ClusterProvision, error) {
	provisionList := &hivev1.ClusterProvisionList{}
	if err := r.List(
		context.TODO(),
		provisionList,
		client.InNamespace(cd.Namespace),
		client.MatchingLabels(map[string]string{constants.ClusterDeploymentNameLabel: cd.Name}),
	); err != nil {
		cdLog.WithError(err).Warn("could not list provisions for clusterdeployment")
		return nil, err
	}
	provisions := make([]*hivev1.ClusterProvision, len(provisionList.Items))
	for i := range provisionList.Items {
		provisions[i] = &provisionList.Items[i]
	}
	sort.Slice(provisions, func(i, j int) bool { return provisions[i].Spec.Attempt < provisions[j].Spec.Attempt })
	return provisions, nil
}

func (r *ReconcileClusterDeployment) getFirstProvision(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) *hivev1.ClusterProvision {
	provisions, err := r.existingProvisions(cd, cdLog)
	if err != nil {
		return nil
	}
	for _, provision := range provisions {
		if provision.Spec.Attempt == 0 {
			return provision
		}
	}
	cdLog.Warn("could not find the first provision for clusterdeployment")
	return nil
}

func (r *ReconcileClusterDeployment) adoptProvision(cd *hivev1.ClusterDeployment, provision *hivev1.ClusterProvision, cdLog log.FieldLogger) error {
	pLog := cdLog.WithField("provision", provision.Name)
	cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: provision.Name}
	if cd.Status.InstallStartedTimestamp == nil {
		n := metav1.Now()
		cd.Status.InstallStartedTimestamp = &n
	}
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		pLog.WithError(err).Log(controllerutils.LogLevel(err), "could not adopt provision")
		return err
	}
	pLog.Info("adopted provision")
	return nil
}

func (r *ReconcileClusterDeployment) deleteStaleProvisions(provs []*hivev1.ClusterProvision, cdLog log.FieldLogger) {
	// Cap the number of existing provisions. Always keep the earliest provision as
	// it is used to determine the total time that it took to install. Take off
	// one extra to make room for the new provision being started.
	amountToDelete := len(provs) - maxProvisions
	if amountToDelete <= 0 {
		return
	}
	cdLog.Infof("Deleting %d old provisions", amountToDelete)
	for _, provision := range provs[1 : amountToDelete+1] {
		pLog := cdLog.WithField("provision", provision.Name)
		pLog.Info("Deleting old provision")
		if err := r.Delete(context.TODO(), provision); err != nil {
			pLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to delete old provision")
		}
	}
}

// deleteOldFailedProvisions deletes the failed provisions which are more than 7 days old
func (r *ReconcileClusterDeployment) deleteOldFailedProvisions(provs []*hivev1.ClusterProvision, cdLog log.FieldLogger) {
	cdLog.Debugf("Deleting failed provisions which are more than 7 days old")
	for _, provision := range provs {
		if provision.Spec.Stage == hivev1.ClusterProvisionStageFailed && time.Since(provision.CreationTimestamp.Time) > (7*24*time.Hour) {
			pLog := cdLog.WithField("provision", provision.Name)
			pLog.Info("Deleting failed provision")
			if err := r.Delete(context.TODO(), provision); err != nil {
				pLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to delete failed provision")
			}
		}
	}
}

// validatePlatformCreds ensure the platform/cloud credentials are at least good enough to authenticate with
func (r *ReconcileClusterDeployment) validatePlatformCreds(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (bool, error) {
	return r.validateCredentialsForClusterDeployment(r.Client, cd, logger)
}

// checkForFailedSync returns true if it finds that the ClusterSync has the Failed condition set
func checkForFailedSync(clusterSync *hiveintv1alpha1.ClusterSync) bool {
	cond := controllerutils.FindCondition(clusterSync.Status.Conditions, hiveintv1alpha1.ClusterSyncFailed)
	if cond != nil {
		return cond.Status == corev1.ConditionTrue
	}
	return false
}

// setSyncSetFailedCondition updates the hivev1.SyncSetFailedCondition
func (r *ReconcileClusterDeployment) setSyncSetFailedCondition(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	var (
		status          corev1.ConditionStatus
		reason, message string
	)
	clusterSync := &hiveintv1alpha1.ClusterSync{}
	switch err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, clusterSync); {
	case apierrors.IsNotFound(err):
		if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused {
			cdLog.Info("SyncSet is paused. ClusterSync will not be created")
			status = corev1.ConditionTrue
			reason = "SyncSetPaused"
			message = "SyncSet is paused. ClusterSync will not be created"
		} else {
			cdLog.Info("ClusterSync has not yet been created")
			status = corev1.ConditionTrue
			reason = "MissingClusterSync"
			message = "ClusterSync has not yet been created"
		}
	case err != nil:
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "could not get ClusterSync")
		return err
	case checkForFailedSync(clusterSync):
		status = corev1.ConditionTrue
		reason = "SyncSetApplyFailure"
		message = "One of the SyncSet applies has failed"
	default:
		status = corev1.ConditionFalse
		reason = "SyncSetApplySuccess"
		message = "SyncSet apply is successful"
	}

	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.SyncSetFailedCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if !changed {
		return nil
	}
	cd.Status.Conditions = conds
	if err := r.Status().Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating syncset failed condition")
		return err
	}
	return nil
}

// addOwnershipToSecret adds cluster deployment as an additional non-controlling owner to secret
func (r *ReconcileClusterDeployment) addOwnershipToSecret(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger, name string) error {
	cdLog = cdLog.WithField("secret", name)

	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: name}, secret); err != nil {
		cdLog.WithError(err).Error("failed to get secret")
		return err
	}

	labelAdded := false

	// Add the label for cluster deployment for reconciling later, and add the owner reference
	if secret.Labels[constants.ClusterDeploymentNameLabel] != cd.Name {
		cdLog.Debug("Setting label on derived object")
		secret.Labels = k8slabels.AddLabel(secret.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
		labelAdded = true
	}

	cdRef := metav1.OwnerReference{
		APIVersion:         cd.APIVersion,
		Kind:               cd.Kind,
		Name:               cd.Name,
		UID:                cd.UID,
		BlockOwnerDeletion: pointer.Bool(true),
	}

	cdRefChanged := librarygocontroller.EnsureOwnerRef(secret, cdRef)
	if cdRefChanged {
		cdLog.Debug("ownership added for cluster deployment")
	}
	if cdRefChanged || labelAdded {
		cdLog.Info("secret has been modified, updating")
		if err := r.Update(context.TODO(), secret); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating secret")
			return err
		}
	}
	return nil
}

// discoverAzureResourceGroup is intended to retrofit Azure clusters which were a) adopted, or b) created before
// support for custom resource groups was added. It attempts to discover the resource group for the cluster by
// - querying the Infrastructure object in the target cluster; and if that goes wrong
// - parsing the install-config
// No op if platform is not Azure.
// No op if the field is already set (we don't validate that it is correct).
// This is best effort. If things go wrong, we do not use the default value. (Note that a parseable install-config
// with the resource group unset is not "wrong" -- we do default in this case.) Consumers of the ResourceGroupName
// are expected to error if it is not set.
// The return indicates whether the field was modified. If `true`, the caller is responsible for pushing
// an Update() back to the server.
func (r *ReconcileClusterDeployment) discoverAzureResourceGroup(cd *hivev1.ClusterDeployment, log log.FieldLogger) bool {
	log = log.WithField("subtask", "discoverAzureResourceGroup")

	// 1) Do we need to do this at all?
	if getClusterPlatform(cd) != constants.PlatformAzure {
		return false
	}
	if !cd.Spec.Installed {
		// Provisioner should set the field when installation finishes; no-op for now.
		return false
	}
	rg, _ := controllerutils.AzureResourceGroup(cd)
	if rg != "" {
		// Already set, nothing to do
		return false
	}

	// Initialize structs as necessary
	if cd.Spec.ClusterMetadata == nil {
		cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
	}
	if cd.Spec.ClusterMetadata.Platform == nil {
		cd.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{}
	}
	if cd.Spec.ClusterMetadata.Platform.Azure == nil {
		cd.Spec.ClusterMetadata.Platform.Azure = &azure.Metadata{}
	}

	// 2) Try the infrastructure object in the remote cluster
	// This little lambda looks up the Infrastructure object in the remote cluster.
	// It returns true iff it successfully set the ResourceGroupName, false otherwise.
	if func() bool {
		remoteClient, unreachable, requeue := remoteclient.ConnectToRemoteCluster(
			cd,
			r.remoteClusterAPIClientBuilder(cd),
			r.Client,
			log,
		)
		if unreachable || requeue || remoteClient == nil {
			// ConnectToRemoteCluster already logged
			return false
		}

		infraObj := &configv1.Infrastructure{}
		if err := remoteClient.Get(context.TODO(), types.NamespacedName{Name: "cluster"}, infraObj); err != nil {
			log.WithError(err).Error("Failed to retrieve infrastructure object from remote cluster")
			return false
		}
		if infraObj.Status.PlatformStatus == nil {
			log.Error("Infrastructure object did not contain PlatformStatus!")
			return false
		}
		if infraObj.Status.PlatformStatus.Azure == nil {
			log.Error("Infrastructure did not contain Azure in PlatformStatus!")
			return false
		}
		rg := infraObj.Status.PlatformStatus.Azure.ResourceGroupName
		if rg == "" {
			log.Error("ResourceGroupName not found in Infrastructure!")
			return false
		}

		log.WithField("resourceGroupName", rg).Info("Found Azure ResourceGroupName in remote cluster's Infrastructure.")
		cd.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName = &rg
		return true
	}() {
		return true
	}
	log.Warning("Failed to discover Azure ResourceGroupName from the remote cluster's Infrastructure object!")

	// 3) If that didn't work for any reason, try the install-config
	if cd.Spec.Provisioning == nil {
		log.Warn("No ClusterDeployment.Spec.Provisioning section")
		// Probably an adopted CD.
		return false
	}
	if cd.Spec.Provisioning.InstallConfigSecretRef == nil || cd.Spec.Provisioning.InstallConfigSecretRef.Name == "" {
		log.Warn("No InstallConfigSecretRef")
		// Also probably an adopted CD (though why would it have Provisioning but not InstallConfigSecretRef?).
		return false
	}
	secretName := cd.Spec.Provisioning.InstallConfigSecretRef.Name
	log = log.WithField("installconfig-secret", secretName)
	icSecret := &corev1.Secret{}
	if err := r.Get(context.Background(),
		types.NamespacedName{
			Namespace: cd.Namespace,
			Name:      secretName,
		},
		icSecret); err != nil {
		log.WithError(err).Error("Failed to load install-config secret. We'll try again later.")
		return false
	}
	ic, err := ValidateInstallConfig(cd, icSecret)
	if err != nil {
		log.WithError(err).Warn("Couldn't validate install-config")
		// This doesn't necessarily mean we didn't unmarshall it, though. Carry on...
	}
	if ic == nil {
		// Today this only happens if unmarshalling fails. Otherwise we've got *something* for an install-config.
		log.Warn("Did not get an install-config from the secret, but also got no error. This should not happen.")
		return false
	}
	if ic.Platform.Azure == nil {
		// This log might be redundant with one from ValidateInstallConfig
		log.Warn("No Azure platform section in install-config")
		return false
	}

	rg = ic.Platform.Azure.ResourceGroupName
	if rg != "" {
		log.WithField("resourceGroupName", rg).Info("Found Azure ResourceGroupName in install-config")
	} else {
		// Default it if possible
		if cd.Spec.ClusterMetadata.InfraID == "" {
			// This shouldn't be possible. InfraID is set during provisioning; and is required
			// for adopted CDs.
			log.Warn("Can't set default Azure ResourceGroup yet: no InfraID set. This should not happen.")
			return false
		}
		// This is the default set by the installer
		rg = fmt.Sprintf("%s-rg", cd.Spec.ClusterMetadata.InfraID)
		log.WithField("resourceGroupName", rg).Info("Azure ResourceGroupName unset in install-config; defaulting")
	}

	// If we get here, rg is nonempty. Use it.
	cd.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName = &rg
	return true
}

// getClusterPlatform returns the platform of a given ClusterDeployment
func getClusterPlatform(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AlibabaCloud != nil:
		return constants.PlatformAlibabaCloud
	case cd.Spec.Platform.AWS != nil:
		return constants.PlatformAWS
	case cd.Spec.Platform.Azure != nil:
		return constants.PlatformAzure
	case cd.Spec.Platform.GCP != nil:
		return constants.PlatformGCP
	case cd.Spec.Platform.OpenStack != nil:
		return constants.PlatformOpenStack
	case cd.Spec.Platform.VSphere != nil:
		return constants.PlatformVSphere
	case cd.Spec.Platform.BareMetal != nil:
		return constants.PlatformBaremetal
	case cd.Spec.Platform.AgentBareMetal != nil:
		return constants.PlatformAgentBaremetal
	case cd.Spec.Platform.IBMCloud != nil:
		return constants.PlatformIBMCloud
	case cd.Spec.Platform.None != nil:
		return constants.PlatformNone
	case cd.Spec.Platform.Ovirt != nil:
		return constants.PlatformOvirt
	}
	return constants.PlatformUnknown
}

// getClusterRegion returns the region of a given ClusterDeployment
func getClusterRegion(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AlibabaCloud != nil:
		return cd.Spec.Platform.AlibabaCloud.Region
	case cd.Spec.Platform.AWS != nil:
		return cd.Spec.Platform.AWS.Region
	case cd.Spec.Platform.Azure != nil:
		return cd.Spec.Platform.Azure.Region
	case cd.Spec.Platform.GCP != nil:
		return cd.Spec.Platform.GCP.Region
	case cd.Spec.Platform.IBMCloud != nil:
		return cd.Spec.Platform.IBMCloud.Region
	}
	return regionUnknown
}

func LoadReleaseImageVerifier(config *rest.Config) (verify.Interface, error) {
	ns := os.Getenv(constants.HiveReleaseImageVerificationConfigMapNamespaceEnvVar)
	name := os.Getenv(constants.HiveReleaseImageVerificationConfigMapNameEnvVar)

	if name == "" {
		return nil, nil
	}
	if ns == "" {
		return nil, errors.New("namespace must be set for Release Image verifier ConfigMap")
	}

	client, err := dynamic.NewForConfig(config) // the verify lib expects unstructured style object for configuration and therefore dynamic makes more sense.
	if err != nil {
		return nil, errors.Wrap(err, "failed to create kube client")
	}

	cm, err := client.Resource(corev1.SchemeGroupVersion.WithResource("configmaps")).
		Namespace(ns).
		Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to read ConfigMap for release image verification")
	}

	// verifier configuration expects the configmap to have certain annotations and
	// ensuring that before passing it on ensures that it is used.
	annos := cm.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos[verify.ReleaseAnnotationConfigMapVerifier] = "true"
	cm.SetAnnotations(annos)

	cmData, err := cm.MarshalJSON()
	if err != nil {
		return nil, err
	}
	m := manifest.Manifest{
		OriginalFilename: "release-image-verifier-configmap",
		GVK:              cm.GroupVersionKind(),
		Obj:              cm,
		Raw:              cmData,
	}
	return verify.NewFromManifests([]manifest.Manifest{m}, sigstore.NewCachedHTTPClientConstructor(sigstore.DefaultClient, nil).HTTPClient)
}

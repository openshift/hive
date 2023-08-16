package clustersync

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName         = hivev1.ClustersyncControllerName
	defaultReapplyInterval = 2 * time.Hour
	reapplyIntervalEnvKey  = "SYNCSET_REAPPLY_INTERVAL"
	reapplyIntervalJitter  = 0.1
	secretAPIVersion       = "v1"
	secretKind             = "Secret"
	labelApply             = "apply"
	labelCreateOrUpdate    = "createOrUpdate"
	labelCreateOnly        = "createOnly"
	metricResultSuccess    = "success"
	metricResultError      = "error"
	stsName                = "hive-clustersync"
)

var (
	metricTimeToApplySyncSet = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_syncset_apply_duration_seconds",
			Help:    "Time to first successfully apply syncset to a cluster",
			Buckets: []float64{5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"group"},
	)
	metricTimeToApplySelectorSyncSet = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_selectorsyncset_apply_duration_seconds",
			Help:    "Time to first successfully apply each SelectorSyncSet to a new cluster. Does not include results when cluster labels change post-install and new syncsets match.",
			Buckets: []float64{5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"name"},
	)

	metricResourcesApplied = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_syncsetinstance_resources_applied_total",
		Help: "Counter incremented each time we sync a resource to a remote cluster, labeled by type of apply and result.",
	},
		[]string{"type", "result"},
	)

	metricTimeToApplySyncSetResource = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_syncsetinstance_apply_duration_seconds",
			Help:    "Time to apply individual resources in a syncset, labeled by type of apply and result.",
			Buckets: []float64{0.5, 1, 3, 5, 10, 20, 30},
		},
		[]string{"type", "result"},
	)

	metricTimeToApplySyncSets = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_clustersync_first_success_duration_seconds",
			Help:    "Time between cluster install complete and all syncsets applied.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
	)
)

func init() {
	metrics.Registry.MustRegister(metricTimeToApplySyncSet)
	metrics.Registry.MustRegister(metricTimeToApplySelectorSyncSet)
	metrics.Registry.MustRegister(metricResourcesApplied)
	metrics.Registry.MustRegister(metricTimeToApplySyncSetResource)
	metrics.Registry.MustRegister(metricTimeToApplySyncSets)
}

// Add creates a new clustersync Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}

	r, err := NewReconciler(mgr, clientRateLimiter)
	if err != nil {
		return err
	}

	logger.Debug("Getting HIVE_CLUSTERSYNC_POD_NAME")
	podname, found := os.LookupEnv("HIVE_CLUSTERSYNC_POD_NAME")

	if !found {
		return errors.New("environment variable HIVE_CLUSTERSYNC_POD_NAME not set")
	}

	logger.Debug("Setting ordinalID")
	parts := strings.Split(podname, "-")
	ordinalIDStr := parts[len(parts)-1]
	ordinalID32, err := strconv.Atoi(ordinalIDStr)
	if err != nil {
		return errors.Wrap(err, "error converting ordinalID to int")
	}

	r.ordinalID = int64(ordinalID32)
	logger.WithField("ordinalID", r.ordinalID).Debug("ordinalID set")

	return AddToManager(mgr, r, concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new ReconcileClusterSync
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*ReconcileClusterSync, error) {
	logger := log.WithField("controller", ControllerName)
	reapplyInterval := defaultReapplyInterval
	if envReapplyInterval := os.Getenv(reapplyIntervalEnvKey); len(envReapplyInterval) > 0 {
		var err error
		reapplyInterval, err = time.ParseDuration(envReapplyInterval)
		if err != nil {
			log.WithError(err).WithField("reapplyInterval", envReapplyInterval).Errorf("unable to parse %s", reapplyIntervalEnvKey)
			return nil, err
		}
	}
	log.WithField("reapplyInterval", reapplyInterval).Info("Reapply interval set")
	c := controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter)
	return &ReconcileClusterSync{
		Client:                c,
		logger:                logger,
		reapplyInterval:       reapplyInterval,
		resourceHelperBuilder: resourceHelperBuilderFunc,
		remoteClusterAPIClientBuilder: func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
			return remoteclient.NewBuilder(c, cd, ControllerName)
		},
	}, nil
}

func resourceHelperBuilderFunc(
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClientBuilderFunc func(cd *hivev1.ClusterDeployment) remoteclient.Builder,
	logger log.FieldLogger,
) (
	resource.Helper,
	error,
) {
	if controllerutils.IsFakeCluster(cd) {
		return resource.NewFakeHelper(logger), nil
	}

	restConfig, err := remoteClusterAPIClientBuilderFunc(cd).RESTConfig()
	if err != nil {
		logger.WithError(err).Error("unable to get REST config")
		return nil, err
	}

	return resource.NewHelperFromRESTConfig(restConfig, logger)
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterSync, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("clusterSync-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, r.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch for changes to SyncSets
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hivev1.SyncSet{}),
		handler.EnqueueRequestsFromMapFunc(requestsForSyncSet)); err != nil {
		return err
	}

	// Watch for changes to SelectorSyncSets
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hivev1.SelectorSyncSet{}),
		handler.EnqueueRequestsFromMapFunc(requestsForSelectorSyncSet(r.Client, r.logger))); err != nil {
		return err
	}

	// Watch for changes to ClusterSync. These have the same name/namespace as the relevant
	// ClusterDeployment, so when a ClusterSync watch triggers, the CD of the same name will be reconciled.
	// When the CD reconciles, it will look up the related ClusterSync.
	if err := c.Watch(source.Kind(mgr.GetCache(), &hiveintv1alpha1.ClusterSync{}), &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch for changes to ClusterSyncLease. These have the same name/namespace as the relevant
	// ClusterDeployment, so when a ClusterSyncLease watch triggers, the CD of the same name will be reconciled.
	// When the CD reconciles, it will look up the related ClusterSyncLease.
	if err := c.Watch(source.Kind(mgr.GetCache(), &hiveintv1alpha1.ClusterSyncLease{}), &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	return nil
}

func requestsForSyncSet(ctx context.Context, o client.Object) []reconcile.Request {
	ss, ok := o.(*hivev1.SyncSet)
	if !ok {
		return nil
	}
	requests := make([]reconcile.Request, len(ss.Spec.ClusterDeploymentRefs))
	for i, cdRef := range ss.Spec.ClusterDeploymentRefs {
		requests[i].Namespace = ss.Namespace
		requests[i].Name = cdRef.Name
	}
	return requests
}

func requestsForSelectorSyncSet(c client.Client, logger log.FieldLogger) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		sss, ok := o.(*hivev1.SelectorSyncSet)
		if !ok {
			return nil
		}
		logger := logger.WithField("selectorSyncSet", sss.Name)
		labelSelector, err := metav1.LabelSelectorAsSelector(&sss.Spec.ClusterDeploymentSelector)
		if err != nil {
			logger.WithError(err).Warn("cannot parse ClusterDeployment selector")
			return nil
		}
		cds := &hivev1.ClusterDeploymentList{}
		if err := c.List(context.Background(), cds, client.MatchingLabelsSelector{Selector: labelSelector}); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not list ClusterDeployments matching SelectorSyncSet")
			return nil
		}
		requests := make([]reconcile.Request, len(cds.Items))
		for i, cd := range cds.Items {
			requests[i].Namespace = cd.Namespace
			requests[i].Name = cd.Name
		}
		return requests
	}
}

var _ reconcile.Reconciler = &ReconcileClusterSync{}

// ReconcileClusterSync reconciles a ClusterDeployment object to apply its SyncSets and SelectorSyncSets
type ReconcileClusterSync struct {
	client.Client
	logger          log.FieldLogger
	reapplyInterval time.Duration

	resourceHelperBuilder func(*hivev1.ClusterDeployment, func(cd *hivev1.ClusterDeployment) remoteclient.Builder, log.FieldLogger) (resource.Helper, error)

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder

	ordinalID int64
}

func (r *ReconcileClusterSync) getAndCheckClusterSyncStatefulSet(logger log.FieldLogger) (*appsv1.StatefulSet, error) {
	hiveNS := controllerutils.GetHiveNamespace()

	logger.Debug("Getting statefulset")
	sts := &appsv1.StatefulSet{}
	err := r.Get(context.Background(), types.NamespacedName{Namespace: hiveNS, Name: stsName}, sts)
	if err != nil {
		logger.WithError(err).WithField("hiveNS", hiveNS).Error("error getting statefulset.")
		return nil, err
	}

	logger.Debug("Ensuring replicas is set")
	if sts.Spec.Replicas == nil {
		return nil, errors.New("sts.Spec.Replicas not set")
	}

	if sts.Status.CurrentReplicas != *sts.Spec.Replicas {
		// This ensures that we don't have partial syncing which may make it seem like things are working.
		// TODO: Remove this once https://issues.redhat.com/browse/CO-1268 is completed as this should no longer be needed.
		return nil, fmt.Errorf("statefulset replica count is off. current: %v  expected: %v", sts.Status.CurrentReplicas, *sts.Spec.Replicas)
	}

	// All checks passed
	return sts, nil
}

// isSyncAssignedToMe determines if this instance of the controller is assigned to the resource being sync'd
func (r *ReconcileClusterSync) isSyncAssignedToMe(sts *appsv1.StatefulSet, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (bool, int64, error) {
	logger.Debug("Getting uid for hashing")
	var uidAsBigInt big.Int

	// There are a couple of assumptions here:
	// * the clusterdeployment uid has 4 sections separated by hyphens
	// * the 4 sections are hex numbers
	// These assumptions are based on the fact that Kubernetes says UIDs are actually
	// ISO/IEC 9834-8 UUIDs. If this changes, this code may fail and may need to be updated.
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
	hexUID := strings.Replace(string(cd.UID), "-", "", 4)
	logger.Debugf("hexUID: %+v", hexUID)
	uidAsBigInt.SetString(hexUID, 16)

	logger.Debug("calculating replicas")
	replicas := int64(*sts.Spec.Replicas)
	// For test purposes, if we've scaled down clustersync so we can run locally, this will be zero; spoof it to one:
	if replicas == 0 {
		logger.Warning("ClusterSync StatefulSet has zero replicas! Hope you're running locally!")
		replicas = 1
	}

	logger.Debug("determining who is assigned to sync this cluster")
	ordinalIDOfAssignee := uidAsBigInt.Mod(&uidAsBigInt, big.NewInt(replicas)).Int64()
	assignedToMe := ordinalIDOfAssignee == r.ordinalID

	logger.WithFields(log.Fields{
		"replicas":            replicas,
		"ordinalIDOfAssignee": ordinalIDOfAssignee,
		"ordinalID":           r.ordinalID,
		"assignedToMe":        assignedToMe,
	}).Debug("computed values")

	return assignedToMe, ordinalIDOfAssignee, nil
}

// Reconcile reads the state of the ClusterDeployment and applies any SyncSets or SelectorSyncSets that need to be
// applied or re-applied.
func (r *ReconcileClusterSync) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	logger.Infof("reconciling ClusterDeployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	recobsrv.SetOutcome(hivemetrics.ReconcileOutcomeNoOp)

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("ClusterDeployment not found")
			return reconcile.Result{}, nil
		}
		log.WithError(err).Error("failed to get ClusterDeployment")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, logger)

	sts, err := r.getAndCheckClusterSyncStatefulSet(logger)
	if err != nil {
		log.WithError(err).Error("failed getting clustersync statefulset")
		return reconcile.Result{}, err
	}

	me, ordinal, err := r.isSyncAssignedToMe(sts, cd, logger)
	if !me || err != nil {
		if err != nil {
			logger.WithError(err).Error("failed determining which instance is assigned to sync this cluster")
			return reconcile.Result{}, err
		}

		logger.Debug("not syncing because isSyncAssignedToMe returned false")
		recobsrv.SetOutcome(hivemetrics.ReconcileOutcomeSkippedSync)
		return reconcile.Result{}, nil
	}

	if controllerutils.IsClusterPausedOrRelocating(cd, logger) {
		return reconcile.Result{}, nil
	}

	if cd.DeletionTimestamp != nil {
		logger.Debug("cluster is being deleted")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		logger.Debug("cluster is not yet installed")
		return reconcile.Result{}, nil
	}

	if unreachable, _ := remoteclient.Unreachable(cd); unreachable {
		logger.Debug("cluster is unreachable")
		return reconcile.Result{}, nil
	}

	// If this cluster carries the fake annotation we will fake out all helper communication with it.
	resourceHelper, err := r.resourceHelperBuilder(cd, r.remoteClusterAPIClientBuilder, logger)
	if err != nil {
		log.WithError(err).Error("cannot create helper")
		return reconcile.Result{}, err
	}

	clusterSync := &hiveintv1alpha1.ClusterSync{}
	switch err := r.Get(context.Background(), request.NamespacedName, clusterSync); {
	case apierrors.IsNotFound(err):
		logger.Info("creating ClusterSync as it does not exist")
		clusterSync.Namespace = cd.Namespace
		clusterSync.Name = cd.Name
		ownerRef := metav1.NewControllerRef(cd, cd.GroupVersionKind())
		ownerRef.Controller = nil
		clusterSync.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		switch err := r.Create(context.Background(), clusterSync); {
		case apierrors.IsAlreadyExists(err):
			// race condition, just proceed
			logger.Warn("race condition: something else has created the clustersync already")
		case err != nil:
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create ClusterSync")
			return reconcile.Result{}, err
		default: // clusterSync was created successfully
			recobsrv.SetOutcome(hivemetrics.ReconcileOutcomeClusterSyncCreated)
			// requeue immediately so that we reconcile soon after the ClusterSync is created
			return reconcile.Result{Requeue: true}, nil
		}
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get ClusterSync")
		return reconcile.Result{}, err
	}

	clusterSync.Status.ControlledByReplica = &ordinal

	needToCreateLease := false
	lease := &hiveintv1alpha1.ClusterSyncLease{}
	switch err := r.Get(context.Background(), request.NamespacedName, lease); {
	case apierrors.IsNotFound(err):
		logger.Info("Lease for ClusterSync does not exist; will need to create")
		needToCreateLease = true
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get lease for ClusterSync")
		return reconcile.Result{}, err
	}

	origStatus := clusterSync.Status.DeepCopy()

	syncSets, err := r.getSyncSetsForClusterDeployment(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	selectorSyncSets, err := r.getSelectorSyncSetsForClusterDeployment(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	needToRenew := r.timeUntilRenew(lease) <= 0

	if needToCreateLease {
		logger.Info("need to reapply all syncsets (missing lease)")
	} else if needToRenew {
		logger.Info("need to reapply all syncsets (time to renew)")
	}

	needToDoFullReapply := needToCreateLease || needToRenew
	recobsrv.SetOutcome(hivemetrics.ReconcileOutcomeFullSync)

	// Apply SyncSets
	syncStatusesForSyncSets, syncSetsNeedRequeue := r.applySyncSets(
		cd,
		"SyncSet",
		syncSets,
		clusterSync.Status.SyncSets,
		needToDoFullReapply,
		false, // no need to report SelectorSyncSet metrics if we're reconciling non-selector SyncSets
		resourceHelper,
		logger,
	)
	clusterSync.Status.SyncSets = syncStatusesForSyncSets

	// Apply SelectorSyncSets
	syncStatusesForSelectorSyncSets, selectorSyncSetsNeedRequeue := r.applySyncSets(
		cd,
		"SelectorSyncSet",
		selectorSyncSets,
		clusterSync.Status.SelectorSyncSets,
		needToDoFullReapply,
		clusterSync.Status.FirstSuccessTime == nil, // only report SelectorSyncSet metrics if we haven't reached first success
		resourceHelper,
		logger,
	)
	clusterSync.Status.SelectorSyncSets = syncStatusesForSelectorSyncSets

	setFailedCondition(clusterSync)

	// Set clusterSync.Status.FirstSyncSetsSuccessTime
	syncStatuses := append(syncStatusesForSyncSets, syncStatusesForSelectorSyncSets...)
	if clusterSync.Status.FirstSuccessTime == nil {
		r.setFirstSuccessTime(syncStatuses, cd, clusterSync, logger)
	}

	// Update the ClusterSync
	if !reflect.DeepEqual(origStatus, &clusterSync.Status) {
		logger.Info("updating ClusterSync")
		if err := r.Status().Update(context.Background(), clusterSync); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterSync")
			return reconcile.Result{}, err
		}
	}

	if needToDoFullReapply {
		logger.Info("setting last full apply time")
		lease.Spec.RenewTime = metav1.NowMicro()
		if needToCreateLease {
			logger.Info("creating lease for ClusterSync")
			lease.Namespace = cd.Namespace
			lease.Name = cd.Name
			ownerRef := metav1.NewControllerRef(clusterSync, clusterSync.GroupVersionKind())
			ownerRef.Controller = nil
			lease.OwnerReferences = []metav1.OwnerReference{*ownerRef}
			if err := r.Create(context.Background(), lease); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create lease for ClusterSync")
				return reconcile.Result{}, err
			}
		} else {
			logger.Info("updating lease for ClusterSync")
			if err := r.Update(context.Background(), lease); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update lease for ClusterSync")
				return reconcile.Result{}, err
			}
		}
	}

	result := reconcile.Result{Requeue: true, RequeueAfter: r.timeUntilRenew(lease)}
	if syncSetsNeedRequeue || selectorSyncSetsNeedRequeue {
		result.RequeueAfter = 0
	}
	return result, nil
}

func (r *ReconcileClusterSync) applySyncSets(
	cd *hivev1.ClusterDeployment,
	syncSetType string,
	syncSets []CommonSyncSet,
	syncStatuses []hiveintv1alpha1.SyncStatus,
	needToDoFullReapply bool,
	reportSelectorSyncSetMetrics bool,
	resourceHelper resource.Helper,
	logger log.FieldLogger,
) (newSyncStatuses []hiveintv1alpha1.SyncStatus, requeue bool) {
	// Sort the syncsets to a consistent ordering. This prevents thrashing in the ClusterSync status due to the order
	// of the syncset status changing from one reconcile to the next.
	sort.Slice(syncSets, func(i, j int) bool {
		return syncSets[i].AsMetaObject().GetName() < syncSets[j].AsMetaObject().GetName()
	})

	deletionList := make([]hiveintv1alpha1.SyncStatus, len(syncStatuses))
	copy(deletionList, syncStatuses)

	for _, syncSet := range syncSets {
		_, indexOfOldStatus := getOldSyncStatus(syncSet, deletionList)
		// Remove the matching old sync status from the slice of sync statuses so that the slice only contains sync
		// statuses that have not been matched to a syncset.
		if indexOfOldStatus >= 0 {
			last := len(deletionList) - 1
			deletionList[indexOfOldStatus] = deletionList[last]
			deletionList = deletionList[:last]
		}
	}

	// sync statuses in the deletionList do not match any syncsets. Any resources to delete in the sync status need to
	// be deleted.
	// We delete old resources before applying new in order to allow resources to be moved from one syncset to
	// another, ex: in the case of a syncset being renamed
	for _, oldSyncStatus := range deletionList {
		remainingResources, err := deleteFromTargetCluster(oldSyncStatus.ResourcesToDelete, nil, resourceHelper, logger)
		if err != nil {
			requeue = true
			newSyncStatus := hiveintv1alpha1.SyncStatus{
				Name:               oldSyncStatus.Name,
				ResourcesToDelete:  remainingResources,
				Result:             hiveintv1alpha1.FailureSyncSetResult,
				FailureMessage:     err.Error(),
				LastTransitionTime: oldSyncStatus.LastTransitionTime,
				FirstSuccessTime:   oldSyncStatus.FirstSuccessTime,
			}
			if !reflect.DeepEqual(oldSyncStatus, newSyncStatus) {
				newSyncStatus.LastTransitionTime = metav1.Now()
			}
			newSyncStatuses = append(newSyncStatuses, newSyncStatus)
		}
	}

	for _, syncSet := range syncSets {
		logger := logger.WithField(syncSetType, syncSet.AsMetaObject().GetName())
		oldSyncStatus, indexOfOldStatus := getOldSyncStatus(syncSet, syncStatuses)

		// Determine if the syncset needs to be applied
		switch {
		case needToDoFullReapply:
			logger.Debug("applying syncset because it is time to do a full re-apply")
		case indexOfOldStatus < 0:
			logger.Debug("applying syncset because the syncset is new")
		case oldSyncStatus.Result != hiveintv1alpha1.SuccessSyncSetResult:
			logger.Debug("applying syncset because the last attempt to apply failed")
		case oldSyncStatus.ObservedGeneration != syncSet.AsMetaObject().GetGeneration():
			logger.Debug("applying syncset because the syncset generation has changed")
		default:
			logger.Debug("skipping apply of syncset since it is up-to-date and it is not time to do a full re-apply")
			newSyncStatuses = append(newSyncStatuses, oldSyncStatus)
			continue
		}

		// Apply the syncset
		resourcesApplied, resourcesInSyncSet, syncSetNeedsRequeue, err := r.applySyncSet(syncSet, resourceHelper, logger)
		newSyncStatus := hiveintv1alpha1.SyncStatus{
			Name:               syncSet.AsMetaObject().GetName(),
			ObservedGeneration: syncSet.AsMetaObject().GetGeneration(),
			Result:             hiveintv1alpha1.SuccessSyncSetResult,
		}
		applyMode := syncSet.GetSpec().ResourceApplyMode
		if applyMode == hivev1.SyncResourceApplyMode {
			newSyncStatus.ResourcesToDelete = resourcesApplied
		}
		// applyMode defaults to UpsertResourceApplyMode
		if (applyMode == hivev1.UpsertResourceApplyMode || applyMode == "") && len(oldSyncStatus.ResourcesToDelete) > 0 {
			logger.Infof("resource apply mode is %v but there are resources to delete in clustersync status", hivev1.UpsertResourceApplyMode)
			oldSyncStatus.ResourcesToDelete = nil
		}
		if err != nil {
			newSyncStatus.Result = hiveintv1alpha1.FailureSyncSetResult
			newSyncStatus.FailureMessage = err.Error()
		}
		if syncSetNeedsRequeue {
			requeue = true
		}

		if indexOfOldStatus >= 0 {
			// Delete any resources that were included in the syncset previously but are no longer included now.
			remainingResources, err := deleteFromTargetCluster(
				oldSyncStatus.ResourcesToDelete,
				func(r hiveintv1alpha1.SyncResourceReference) bool {
					return !containsResource(resourcesInSyncSet, r)
				},
				resourceHelper,
				logger,
			)
			if err != nil {
				requeue = true
				newSyncStatus.Result = hiveintv1alpha1.FailureSyncSetResult
				if newSyncStatus.FailureMessage != "" {
					newSyncStatus.FailureMessage += "\n"
				}
				newSyncStatus.FailureMessage += err.Error()
			}
			newSyncStatus.ResourcesToDelete = mergeResources(newSyncStatus.ResourcesToDelete, remainingResources)

			newSyncStatus.LastTransitionTime = oldSyncStatus.LastTransitionTime
			newSyncStatus.FirstSuccessTime = oldSyncStatus.FirstSuccessTime
		}

		// Update the last transition time if there were any changes to the sync status.
		if !reflect.DeepEqual(oldSyncStatus, newSyncStatus) {
			newSyncStatus.LastTransitionTime = metav1.Now()
		}

		// Set the FirstSuccessTime if this is the first success. Also, observe the apply-duration metric.
		if newSyncStatus.Result == hiveintv1alpha1.SuccessSyncSetResult && oldSyncStatus.FirstSuccessTime == nil {
			now := metav1.Now()
			newSyncStatus.FirstSuccessTime = &now
			startTime := syncSet.AsMetaObject().GetCreationTimestamp().Time
			if cd.Status.InstalledTimestamp != nil && startTime.Before(cd.Status.InstalledTimestamp.Time) {
				startTime = cd.Status.InstalledTimestamp.Time
			}
			if cond := controllerutils.FindCondition(cd.Status.Conditions, hivev1.UnreachableCondition); cond != nil && startTime.Before(cond.LastTransitionTime.Time) {
				startTime = cond.LastTransitionTime.Time
			}
			applyTime := now.Sub(startTime).Seconds()

			if syncSet.AsMetaObject().GetNamespace() == "" {
				if reportSelectorSyncSetMetrics {
					// Report SelectorSyncSet metric *only if* we have not yet reached our "everything has applied successfully for the first time"
					// state. This is to handle situations where a clusters labels change long after it was installed, resulting
					// in a new SelectorSyncSet matching, and a metric showing days/weeks/months time to first apply. In this scenario
					// we have no idea when the label was added, and thus it is not currently possible to report the time from
					// label added to SyncSet successfully applied.
					logger.WithField("applyTime", applyTime).Debug("observed first successful apply of SelectorSyncSet for cluster")
					metricTimeToApplySelectorSyncSet.WithLabelValues(syncSet.AsMetaObject().GetName()).Observe(applyTime)
				} else {
					logger.Info("skipped observing first successful apply of SelectorSyncSet metric because ClusterSync has a FirstSuccessTime")
				}
			} else {
				// For non-selector SyncSets we have a more accurate startTime, either ClusterDeployment installedTimestamp
				// or SyncSet creationTimestamp.
				logger.WithField("applyTime", applyTime).Debug("observed first successful apply of SyncSet for cluster")
				if syncSetGroup, ok := syncSet.AsMetaObject().GetAnnotations()[constants.SyncSetMetricsGroupAnnotation]; ok && syncSetGroup != "" {
					metricTimeToApplySyncSet.WithLabelValues(syncSetGroup).Observe(applyTime)
				} else {
					metricTimeToApplySyncSet.WithLabelValues("none").Observe(applyTime)
				}
			}
		}

		// Sort ResourcesToDelete to prevent update thrashing.
		sort.Slice(newSyncStatus.ResourcesToDelete, func(i, j int) bool {
			return orderResources(newSyncStatus.ResourcesToDelete[i], newSyncStatus.ResourcesToDelete[j])
		})
		newSyncStatuses = append(newSyncStatuses, newSyncStatus)
	}

	return
}

func getOldSyncStatus(syncSet CommonSyncSet, syncSetStatuses []hiveintv1alpha1.SyncStatus) (hiveintv1alpha1.SyncStatus, int) {
	for i, status := range syncSetStatuses {
		if status.Name == syncSet.AsMetaObject().GetName() {
			return syncSetStatuses[i], i
		}
	}
	return hiveintv1alpha1.SyncStatus{}, -1
}

func (r *ReconcileClusterSync) applySyncSet(
	syncSet CommonSyncSet,
	resourceHelper resource.Helper,
	logger log.FieldLogger,
) (
	resourcesApplied []hiveintv1alpha1.SyncResourceReference,
	resourcesInSyncSet []hiveintv1alpha1.SyncResourceReference,
	requeue bool,
	returnErr error,
) {
	resources, referencesToResources, decodeErr := decodeResources(syncSet, logger)
	referencesToSecrets := referencesToSecrets(syncSet)
	resourcesInSyncSet = append(referencesToResources, referencesToSecrets...)
	if decodeErr != nil {
		returnErr = decodeErr
		return
	}

	applyFn := resourceHelper.Apply
	applyFnMetricsLabel := labelApply
	switch syncSet.GetSpec().ApplyBehavior {
	case hivev1.CreateOrUpdateSyncSetApplyBehavior:
		applyFn = resourceHelper.CreateOrUpdate
		applyFnMetricsLabel = labelCreateOrUpdate
	case hivev1.CreateOnlySyncSetApplyBehavior:
		applyFn = resourceHelper.Create
		applyFnMetricsLabel = labelCreateOnly
	}

	// Apply Resources
	for i, resource := range resources {
		returnErr, requeue = r.applyResource(i, resource, referencesToResources[i], applyFn, applyFnMetricsLabel, logger)
		if returnErr != nil {
			resourcesApplied = referencesToResources[:i]
			return
		}
	}
	resourcesApplied = referencesToResources

	// Apply Secrets
	for i, secretMapping := range syncSet.GetSpec().Secrets {
		returnErr, requeue = r.applySecret(syncSet, i, secretMapping, referencesToSecrets[i], applyFn, applyFnMetricsLabel, logger)
		if returnErr != nil {
			resourcesApplied = append(resourcesApplied, referencesToSecrets[:i]...)
			return
		}
	}
	resourcesApplied = append(resourcesApplied, referencesToSecrets...)

	// Apply Patches
	for i, patch := range syncSet.GetSpec().Patches {
		returnErr, requeue = r.applyPatch(i, patch, resourceHelper, logger)
		if returnErr != nil {
			return
		}
	}

	logger.Info("syncset applied")
	return
}

func decodeResources(syncSet CommonSyncSet, logger log.FieldLogger) (
	resources []*unstructured.Unstructured, references []hiveintv1alpha1.SyncResourceReference, returnErr error,
) {
	var decodeErrors []error
	for i, resource := range syncSet.GetSpec().Resources {
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(resource.Raw, u); err != nil {
			logger.WithField("resourceIndex", i).WithError(err).Warn("error decoding unstructured object")
			decodeErrors = append(decodeErrors, errors.Wrapf(err, "failed to decode resource %d", i))
			continue
		}
		resources = append(resources, u)
		references = append(references, hiveintv1alpha1.SyncResourceReference{
			APIVersion: u.GetAPIVersion(),
			Kind:       u.GetKind(),
			Namespace:  u.GetNamespace(),
			Name:       u.GetName(),
		})
	}
	returnErr = utilerrors.NewAggregate(decodeErrors)
	return
}

func referencesToSecrets(syncSet CommonSyncSet) []hiveintv1alpha1.SyncResourceReference {
	var references []hiveintv1alpha1.SyncResourceReference
	for _, secretMapping := range syncSet.GetSpec().Secrets {
		references = append(references, hiveintv1alpha1.SyncResourceReference{
			APIVersion: secretAPIVersion,
			Kind:       secretKind,
			Namespace:  secretMapping.TargetRef.Namespace,
			Name:       secretMapping.TargetRef.Name,
		})
	}
	return references
}

func (r *ReconcileClusterSync) applyResource(
	resourceIndex int,
	resource *unstructured.Unstructured,
	reference hiveintv1alpha1.SyncResourceReference,
	applyFn func(obj []byte) (resource.ApplyResult, error),
	applyFnMetricsLabel string,
	logger log.FieldLogger,
) (returnErr error, requeue bool) {
	logger = logger.WithField("resourceIndex", resourceIndex).
		WithField("resourceNamespace", reference.Namespace).
		WithField("resourceName", reference.Name).
		WithField("resourceAPIVersion", reference.APIVersion).
		WithField("resourceKind", reference.Kind)
	logger.Debug("applying resource")
	if err := applyToTargetCluster(resource, applyFnMetricsLabel, applyFn, logger); err != nil {
		return errors.Wrapf(err, "failed to apply resource %d", resourceIndex), true
	}
	return nil, false
}

func (r *ReconcileClusterSync) applySecret(
	syncSet CommonSyncSet,
	secretIndex int,
	secretMapping hivev1.SecretMapping,
	reference hiveintv1alpha1.SyncResourceReference,
	applyFn func(obj []byte) (resource.ApplyResult, error),
	applyFnMetricsLabel string,
	logger log.FieldLogger,
) (returnErr error, requeue bool) {
	logger = logger.WithField("secretIndex", secretIndex).
		WithField("secretNamespace", reference.Namespace).
		WithField("secretName", reference.Name)
	syncSetNamespace := syncSet.AsMetaObject().GetNamespace()
	srcNamespace := secretMapping.SourceRef.Namespace
	if srcNamespace == "" {
		// The namespace of the source secret is required for SelectorSyncSets.
		if syncSetNamespace == "" {
			logger.Warn("namespace must be specified for source secret")
			return fmt.Errorf("source namespace missing for secret %d", secretIndex), false
		}
		// Use the namespace of the SyncSet if the namespace of the source secret is omitted.
		srcNamespace = syncSetNamespace
	} else {
		// If the namespace of the source secret is specified, then it must match the namespace of the SyncSet.
		if syncSetNamespace != "" && syncSetNamespace != srcNamespace {
			logger.Warn("source secret must be in same namespace as SyncSet")
			return fmt.Errorf("source in wrong namespace for secret %d", secretIndex), false
		}
	}
	secret := &corev1.Secret{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: srcNamespace, Name: secretMapping.SourceRef.Name}, secret); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "cannot read secret")
		return errors.Wrapf(err, "failed to read secret %d", secretIndex), true
	}
	// Clear out the fields of the metadata which are specific to the cluster to which the secret belongs.
	secret.ObjectMeta = metav1.ObjectMeta{
		Namespace:   secretMapping.TargetRef.Namespace,
		Name:        secretMapping.TargetRef.Name,
		Annotations: secret.Annotations,
		Labels:      secret.Labels,
	}
	logger.Debug("applying secret")
	if err := applyToTargetCluster(secret, applyFnMetricsLabel, applyFn, logger); err != nil {
		return errors.Wrapf(err, "failed to apply secret %d", secretIndex), true
	}
	return nil, false
}

func (r *ReconcileClusterSync) applyPatch(
	patchIndex int,
	patch hivev1.SyncObjectPatch,
	resourceHelper resource.Helper,
	logger log.FieldLogger,
) (returnErr error, requeue bool) {
	logger = logger.WithField("patchIndex", patchIndex).
		WithField("patchNamespace", patch.Namespace).
		WithField("patchName", patch.Name).
		WithField("patchAPIVersion", patch.APIVersion).
		WithField("patchKind", patch.Kind)
	logger.Debug("applying patch")
	if err := resourceHelper.Patch(
		types.NamespacedName{Namespace: patch.Namespace, Name: patch.Name},
		patch.Kind,
		patch.APIVersion,
		[]byte(patch.Patch),
		patch.PatchType,
	); err != nil {
		return errors.Wrapf(err, "failed to apply patch %d", patchIndex), true
	}
	return nil, false
}

func applyToTargetCluster(
	obj hivev1.MetaRuntimeObject,
	applyFnMetricLabel string,
	applyFn func(obj []byte) (resource.ApplyResult, error),
	logger log.FieldLogger,
) error {
	startTime := time.Now()
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	// Inject the hive managed annotation to help end-users see that a resource is managed by hive:
	labels[constants.HiveManagedLabel] = "true"
	obj.SetLabels(labels)

	bytes, err := json.Marshal(obj)
	if err != nil {
		logger.WithError(err).Error("error marshalling unstructured object to json bytes")
		return err
	}

	applyResult, err := applyFn(bytes)
	// Record the amount of time we took to apply this specific resource. When combined with the metric for duration of
	// our kube client requests, we can get an idea how much time we're spending cpu bound vs network bound.
	applyTime := metav1.Now().Sub(startTime).Seconds()
	if err != nil {
		logger.WithError(err).Warn("error applying resource")
		metricResourcesApplied.WithLabelValues(applyFnMetricLabel, metricResultError).Inc()
		metricTimeToApplySyncSetResource.WithLabelValues(applyFnMetricLabel, metricResultError).Observe(applyTime)
	} else {
		logger.WithField("applyResult", applyResult).Debug("resource applied")
		metricResourcesApplied.WithLabelValues(applyFnMetricLabel, metricResultSuccess).Inc()
		metricTimeToApplySyncSetResource.WithLabelValues(applyFnMetricLabel, metricResultSuccess).Observe(applyTime)
	}
	return err
}

func deleteFromTargetCluster(
	resources []hiveintv1alpha1.SyncResourceReference,
	shouldDelete func(hiveintv1alpha1.SyncResourceReference) bool,
	resourceHelper resource.Helper,
	logger log.FieldLogger,
) (remainingResources []hiveintv1alpha1.SyncResourceReference, returnErr error) {
	var allErrs []error
	for _, r := range resources {
		if shouldDelete != nil && !shouldDelete(r) {
			remainingResources = append(remainingResources, r)
			continue
		}
		logger := logger.WithField("resourceNamespace", r.Namespace).
			WithField("resourceName", r.Name).
			WithField("resourceAPIVersion", r.APIVersion).
			WithField("resourceKind", r.Kind)
		logger.Info("deleting resource")
		if err := resourceHelper.Delete(r.APIVersion, r.Kind, r.Namespace, r.Name); err != nil {
			logger.WithError(err).Warn("could not delete resource")
			allErrs = append(allErrs, fmt.Errorf("failed to delete %s, Kind=%s %s/%s: %w", r.APIVersion, r.Kind, r.Namespace, r.Name, err))
			remainingResources = append(remainingResources, r)
		}
	}
	return remainingResources, utilerrors.NewAggregate(allErrs)
}

func (r *ReconcileClusterSync) getSyncSetsForClusterDeployment(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]CommonSyncSet, error) {
	syncSetsList := &hivev1.SyncSetList{}
	if err := r.List(context.Background(), syncSetsList, client.InNamespace(cd.Namespace)); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not list SyncSets")
		return nil, err
	}
	var syncSets []CommonSyncSet
	for i, ss := range syncSetsList.Items {
		if !doesSyncSetApplyToClusterDeployment(&ss, cd) {
			continue
		}
		syncSets = append(syncSets, (*SyncSetAsCommon)(&syncSetsList.Items[i]))
	}
	return syncSets, nil
}

func (r *ReconcileClusterSync) getSelectorSyncSetsForClusterDeployment(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]CommonSyncSet, error) {
	selectorSyncSetsList := &hivev1.SelectorSyncSetList{}
	if err := r.List(context.Background(), selectorSyncSetsList); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not list SelectorSyncSets")
		return nil, err
	}
	var selectorSyncSets []CommonSyncSet
	for i, sss := range selectorSyncSetsList.Items {
		if !doesSelectorSyncSetApplyToClusterDeployment(&sss, cd, logger) {
			continue
		}
		selectorSyncSets = append(selectorSyncSets, (*SelectorSyncSetAsCommon)(&selectorSyncSetsList.Items[i]))
	}
	return selectorSyncSets, nil
}

func doesSyncSetApplyToClusterDeployment(syncSet *hivev1.SyncSet, cd *hivev1.ClusterDeployment) bool {
	for _, cdRef := range syncSet.Spec.ClusterDeploymentRefs {
		if cdRef.Name == cd.Name {
			return true
		}
	}
	return false
}

func doesSelectorSyncSetApplyToClusterDeployment(selectorSyncSet *hivev1.SelectorSyncSet, cd *hivev1.ClusterDeployment, logger log.FieldLogger) bool {
	labelSelector, err := metav1.LabelSelectorAsSelector(&selectorSyncSet.Spec.ClusterDeploymentSelector)
	if err != nil {
		logger.WithError(err).Error("unable to convert selector")
		return false
	}
	return labelSelector.Matches(labels.Set(cd.Labels))
}

func setFailedCondition(clusterSync *hiveintv1alpha1.ClusterSync) {
	status := corev1.ConditionFalse
	reason := "Success"
	message := "All SyncSets and SelectorSyncSets have been applied to the cluster"
	failingSyncSets := getFailingSyncSets(clusterSync.Status.SyncSets)
	failingSelectorSyncSets := getFailingSyncSets(clusterSync.Status.SelectorSyncSets)
	if len(failingSyncSets)+len(failingSelectorSyncSets) != 0 {
		status = corev1.ConditionTrue
		reason = "Failure"
		var failureNames []string
		if len(failingSyncSets) != 0 {
			failureNames = append(failureNames, namesForFailureMessage("SyncSet", failingSyncSets))
		}
		if len(failingSelectorSyncSets) != 0 {
			failureNames = append(failureNames, namesForFailureMessage("SelectorSyncSet", failingSelectorSyncSets))
		}
		verb := "is"
		if len(failingSyncSets)+len(failingSelectorSyncSets) > 1 {
			verb = "are"
		}
		message = fmt.Sprintf("%s %s failing", strings.Join(failureNames, " and "), verb)
	}
	if len(clusterSync.Status.Conditions) > 0 {
		cond := clusterSync.Status.Conditions[0]
		if status == cond.Status &&
			reason == cond.Reason &&
			message == cond.Message {
			return
		}
	}
	clusterSync.Status.Conditions = []hiveintv1alpha1.ClusterSyncCondition{{
		Type:               hiveintv1alpha1.ClusterSyncFailed,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}}
}

func getFailingSyncSets(syncStatuses []hiveintv1alpha1.SyncStatus) []string {
	var failures []string
	for _, status := range syncStatuses {
		if status.Result != hiveintv1alpha1.SuccessSyncSetResult {
			failures = append(failures, status.Name)
		}
	}
	return failures
}

func (r *ReconcileClusterSync) setFirstSuccessTime(syncStatuses []hiveintv1alpha1.SyncStatus, cd *hivev1.ClusterDeployment, clusterSync *hiveintv1alpha1.ClusterSync, logger log.FieldLogger) {
	if cd.Status.InstalledTimestamp == nil {
		return
	}
	lastSuccessTime := &metav1.Time{}
	for _, status := range syncStatuses {
		if status.FirstSuccessTime == nil {
			return
		}
		if status.FirstSuccessTime.Time.After(lastSuccessTime.Time) {
			lastSuccessTime = status.FirstSuccessTime
		}
	}
	// When len(syncStatuses) == 0, meaning there are no syncsets which apply to the cluster, we will use now as the last success time
	if len(syncStatuses) == 0 {
		now := metav1.Now()
		lastSuccessTime = &now
	}
	clusterSync.Status.FirstSuccessTime = lastSuccessTime
	allSyncSetsAppliedDuration := lastSuccessTime.Time.Sub(cd.Status.InstalledTimestamp.Time)
	logger.Infof("observed syncsets applied duration: %v seconds", allSyncSetsAppliedDuration.Seconds())
	metricTimeToApplySyncSets.Observe(float64(allSyncSetsAppliedDuration.Seconds()))
}

func namesForFailureMessage(syncSetKind string, names []string) string {
	if len(names) > 1 {
		syncSetKind += "s"
	}
	return fmt.Sprintf("%s %s", syncSetKind, strings.Join(names, ", "))
}

func mergeResources(a, b []hiveintv1alpha1.SyncResourceReference) []hiveintv1alpha1.SyncResourceReference {
	if len(a) == 0 {
		return b
	}
	for _, r := range b {
		if !containsResource(a, r) {
			a = append(a, r)
		}
	}
	return a
}

func containsResource(resources []hiveintv1alpha1.SyncResourceReference, resource hiveintv1alpha1.SyncResourceReference) bool {
	for _, r := range resources {
		if r == resource {
			return true
		}
	}
	return false
}

func orderResources(a, b hiveintv1alpha1.SyncResourceReference) bool {
	if x, y := a.APIVersion, b.APIVersion; x != y {
		return x < y
	}
	if x, y := a.Kind, b.Kind; x != y {
		return x < y
	}
	if x, y := a.Namespace, b.Namespace; x != y {
		return x < y
	}
	return a.Name < b.Name
}

func (r *ReconcileClusterSync) timeUntilRenew(lease *hiveintv1alpha1.ClusterSyncLease) time.Duration {
	timeUntilNext := r.reapplyInterval - time.Since(lease.Spec.RenewTime.Time) +
		time.Duration(reapplyIntervalJitter*rand.Float64()*r.reapplyInterval.Seconds())*time.Second
	if timeUntilNext < 0 {
		return 0
	}
	return timeUntilNext
}

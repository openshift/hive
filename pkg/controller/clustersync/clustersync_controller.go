package clustersync

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/pkg/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName         = "clustersync"
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
)

var (
	metricTimeToApplySyncSet = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_syncset_apply_duration_seconds",
			Help:    "Time to first successfully apply syncset to a cluster",
			Buckets: []float64{5, 10, 30, 60, 300, 600, 1800, 3600},
		},
	)
	metricTimeToApplySelectorSyncSet = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_selectorsyncset_apply_duration_seconds",
			Help:    "Time to first successfully apply selector syncset to a cluster",
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
)

func init() {
	metrics.Registry.MustRegister(metricTimeToApplySyncSet)
	metrics.Registry.MustRegister(metricTimeToApplySelectorSyncSet)
	metrics.Registry.MustRegister(metricResourcesApplied)
	metrics.Registry.MustRegister(metricTimeToApplySyncSetResource)
}

// Add creates a new clustersync Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := NewReconciler(mgr)
	if err != nil {
		return err
	}
	return AddToManager(mgr, r)
}

// NewReconciler returns a new ReconcileClusterSync
func NewReconciler(mgr manager.Manager) (*ReconcileClusterSync, error) {
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
	c := controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName)
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

func resourceHelperBuilderFunc(restConfig *rest.Config, logger log.FieldLogger) (resource.Helper, error) {
	return resource.NewHelperFromRESTConfig(restConfig, logger), nil
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterSync) error {
	// Create a new controller
	c, err := controller.New("clusterSync-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch for changes to SyncSets
	if err := c.Watch(
		&source.Kind{Type: &hivev1.SyncSet{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(requestsForSyncSet),
		},
	); err != nil {
		return err
	}

	// Watch for changes to SelectorSyncSets
	if err := c.Watch(
		&source.Kind{Type: &hivev1.SelectorSyncSet{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: requestsForSelectorSyncSet(r.Client, r.logger),
		},
	); err != nil {
		return err
	}

	// Watch for creates of ClusterSyncs. The controller only watches creates because there is not spec and no benefit
	// to requeueing when the status changes.
	if err := c.Watch(
		&source.Kind{Type: &hiveintv1alpha1.ClusterSync{}},
		handler.Funcs{
			CreateFunc: func(e event.CreateEvent, q workqueue.RateLimitingInterface) {
				clusterSync, ok := e.Object.(*hiveintv1alpha1.ClusterSync)
				if !ok {
					return
				}
				// ClusterSync has the same name as the ClusterDeployment
				cdKey := client.ObjectKey{Namespace: clusterSync.Namespace, Name: clusterSync.Name}.String()
				q.Add(cdKey)
			},
		},
	); err != nil {
		return err
	}

	return nil
}

func requestsForSyncSet(o handler.MapObject) []reconcile.Request {
	ss, ok := o.Object.(*hivev1.SyncSet)
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

func requestsForSelectorSyncSet(c client.Client, logger log.FieldLogger) handler.ToRequestsFunc {
	return func(o handler.MapObject) []reconcile.Request {
		sss, ok := o.Object.(*hivev1.SelectorSyncSet)
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

	resourceHelperBuilder func(*rest.Config, log.FieldLogger) (resource.Helper, error)

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile reads the state of the ClusterDeployment and applies any SyncSets or SelectorSyncSets that need to be
// applied or re-applied.
func (r *ReconcileClusterSync) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("ClusterDeployment", request.NamespacedName)

	logger.Infof("reconciling ClusterDeployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

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

	if cd.DeletionTimestamp != nil {
		logger.Debug("cluster is being deleted")
		return reconcile.Result{}, nil
	}

	if unreachable, _ := remoteclient.Unreachable(cd); unreachable {
		logger.Debug("cluster is unreachable")
		return reconcile.Result{}, nil
	}

	restConfig, err := r.remoteClusterAPIClientBuilder(cd).RESTConfig()
	if err != nil {
		logger.WithError(err).Error("unable to get REST config")
		return reconcile.Result{}, err
	}
	resourceHelper, err := r.resourceHelperBuilder(restConfig, logger)
	if err != nil {
		log.WithError(err).Error("cannot create helper")
		return reconcile.Result{}, err
	}

	needToCreateClusterSync := false
	clusterSync := &hiveintv1alpha1.ClusterSync{}
	switch err := r.Get(context.Background(), request.NamespacedName, clusterSync); {
	case apierrors.IsNotFound(err):
		logger.Info("creating ClusterSync as it does not exist")
		clusterSync.Namespace = cd.Namespace
		clusterSync.Name = cd.Name
		ownerRef := metav1.NewControllerRef(cd, cd.GroupVersionKind())
		ownerRef.Controller = nil
		clusterSync.OwnerReferences = []metav1.OwnerReference{*ownerRef}
		if err := r.Create(context.Background(), clusterSync); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create ClusterSync")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get ClusterSync")
		return reconcile.Result{}, err
	}

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

	needToDoFullReapply := needToCreateClusterSync || r.timeUntilFullReapply(lease) <= 0
	if needToDoFullReapply {
		logger.Info("need to reapply all syncsets")
	}

	// Apply SyncSets
	syncStatusesForSyncSets, syncSetsNeedRequeue := r.applySyncSets(
		cd,
		"SyncSet",
		syncSets,
		clusterSync.Status.SyncSets,
		needToDoFullReapply,
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
		resourceHelper,
		logger,
	)
	clusterSync.Status.SelectorSyncSets = syncStatusesForSelectorSyncSets

	setFailedCondition(clusterSync)

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

	result := reconcile.Result{Requeue: true, RequeueAfter: r.timeUntilFullReapply(lease)}
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
	resourceHelper resource.Helper,
	logger log.FieldLogger,
) (newSyncStatuses []hiveintv1alpha1.SyncStatus, requeue bool) {
	// Sort the syncsets to a consistent ordering. This prevents thrashing in the ClusterSync status due to the order
	// of the syncset status changing from one reconcile to the next.
	sort.Slice(syncSets, func(i, j int) bool {
		return syncSets[i].AsMetaObject().GetName() < syncSets[j].AsMetaObject().GetName()
	})

	for _, syncSet := range syncSets {
		logger := logger.WithField(syncSetType, syncSet.AsMetaObject().GetName())
		oldSyncStatus, indexOfOldStatus := getOldSyncStatus(syncSet, syncStatuses)
		// Remove the matching old sync status from the slice of sync statuses so that the slice only contains sync
		// statuses that have not been matched to a syncset.
		if indexOfOldStatus >= 0 {
			last := len(syncStatuses) - 1
			syncStatuses[indexOfOldStatus] = syncStatuses[last]
			syncStatuses = syncStatuses[:last]
		}

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
		if syncSet.GetSpec().ResourceApplyMode == hivev1.SyncResourceApplyMode {
			newSyncStatus.ResourcesToDelete = resourcesApplied
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
			if cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition); cond != nil && startTime.Before(cond.LastTransitionTime.Time) {
				startTime = cond.LastTransitionTime.Time
			}
			applyTime := now.Sub(startTime).Seconds()
			if syncSet.AsMetaObject().GetNamespace() == "" {
				metricTimeToApplySelectorSyncSet.WithLabelValues(syncSet.AsMetaObject().GetName()).Observe(applyTime)
			} else {
				metricTimeToApplySyncSet.Observe(applyTime)
			}
		}

		// Sort ResourcesToDelete to prevent update thrashing.
		sort.Slice(newSyncStatus.ResourcesToDelete, func(i, j int) bool {
			return orderResources(newSyncStatus.ResourcesToDelete[i], newSyncStatus.ResourcesToDelete[j])
		})
		newSyncStatuses = append(newSyncStatuses, newSyncStatus)
	}

	// The remaining sync statuses in syncStatuses do not match any syncsets. Any resources to delete in the sync status
	// need to be deleted.
	for _, oldSyncStatus := range syncStatuses {
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
		returnErr, requeue = r.applyResource(syncSet, i, resource, referencesToResources[i], applyFn, applyFnMetricsLabel, logger)
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
	syncSet CommonSyncSet,
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
	if err := applyToTargetCluster(syncSet, resource, applyFnMetricsLabel, applyFn, logger); err != nil {
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
	if err := applyToTargetCluster(syncSet, secret, applyFnMetricsLabel, applyFn, logger); err != nil {
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
	// TODO: label and annotate these?
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

// ensureHiveSourceAnnotation makes sure obj has a HiveSourcesAnnotation containing an
// appropriate value for syncSet.
// Currently the annotation is a space-separated list of strings of the form Kind/Name,
// where Kind is either SyncSet or SelectorSyncSet, and Name is the name of the
// [Selector]SyncSet.
func ensureHiveSourceAnnotation(obj hivev1.MetaRuntimeObject, syncSet CommonSyncSet) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	hsa := strings.Split(annotations[constants.HiveSourcesAnnotation], " ")
	source := syncSet.GetSourceAnnotation()
	for _, s := range hsa {
		if s == source {
			return
		}
	}
	hsa = append(hsa, source)
	annotations[constants.HiveSourcesAnnotation] = strings.Join(hsa, " ")
	obj.SetAnnotations(annotations)
}

func applyToTargetCluster(
	syncSet CommonSyncSet,
	obj hivev1.MetaRuntimeObject,
	applyFnMetricLabel string,
	applyFn func(obj []byte) (resource.ApplyResult, error),
	logger log.FieldLogger,
) error {
	startTime := time.Now()
	// Inject the hive managed label and annotation to help end-users see how the resource is managed by hive:
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	labels[constants.HiveManagedLabel] = "true"
	obj.SetLabels(labels)
	ensureHiveSourceAnnotation(obj, syncSet)

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
			allErrs = append(allErrs, fmt.Errorf("Failed to delete %s, Kind=%s %s/%s: %w", r.APIVersion, r.Kind, r.Namespace, r.Name, err))
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

func (r *ReconcileClusterSync) timeUntilFullReapply(lease *hiveintv1alpha1.ClusterSyncLease) time.Duration {
	timeUntilNext := r.reapplyInterval - time.Since(lease.Spec.RenewTime.Time) +
		time.Duration(reapplyIntervalJitter*rand.Float64()*r.reapplyInterval.Seconds())*time.Second
	if timeUntilNext < 0 {
		return 0
	}
	return timeUntilNext
}

package clustersyncset

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/yaml"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName         = "clustersyncset"
	defaultReapplyInterval = 2 * time.Hour
	reapplyIntervalEnvKey  = "SYNCSET_REAPPLY_INTERVAL"
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

// Add creates a new clustersyncset Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r, err := NewReconciler(mgr)
	if err != nil {
		return err
	}
	return AddToManager(mgr, r)
}

// NewReconciler returns a new ReconcileClusterSyncSet
func NewReconciler(mgr manager.Manager) (*ReconcileClusterSyncSet, error) {
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
	return &ReconcileClusterSyncSet{
		Client:          c,
		logger:          logger,
		reapplyInterval: reapplyInterval,
		applierBuilder:  applierBuilderFunc,
		remoteClusterAPIClientBuilder: func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
			return remoteclient.NewBuilder(c, cd, ControllerName)
		},
	}, nil
}

// applierBuilderFunc returns an Applier which implements Info, Apply and Patch
func applierBuilderFunc(restConfig *rest.Config, logger log.FieldLogger) applier {
	return resource.NewHelperFromRESTConfig(restConfig, logger)
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterSyncSet) error {
	// Create a new controller
	c, err := controller.New("clustersyncset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
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

var _ reconcile.Reconciler = &ReconcileClusterSyncSet{}

// ReconcileClusterSyncSet reconciles a ClusterDeployment object to apply its SyncSets and SelectorSyncSets
type ReconcileClusterSyncSet struct {
	client.Client
	logger          log.FieldLogger
	reapplyInterval time.Duration

	applierBuilder func(*rest.Config, log.FieldLogger) applier

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile reads the state of the ClusterDeployment and applies any SyncSets or SelectorSyncSets that need to be
// applied or re-applied.
func (r *ReconcileClusterSyncSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("ClusterDeployment", request.Name)

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
		// Error reading the object - requeue the request.
		log.WithError(err).Error("error reading ClusterDeployment")
		return reconcile.Result{}, err
	}

	// If the ClusterDeployment has been deleted, there is nothing to do.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if unreachable, _ := remoteclient.Unreachable(cd); unreachable {
		return reconcile.Result{}, nil
	}

	restConfig, err := r.remoteClusterAPIClientBuilder(cd).RESTConfig()
	if err != nil {
		logger.WithError(err).Error("unable to get REST config")
		return reconcile.Result{}, err
	}
	applier := r.applierBuilder(restConfig, logger)

	needToCreate := false
	clusterSyncSet := &hivev1.ClusterSyncSet{}
	switch err := r.Get(context.Background(), request.NamespacedName, clusterSyncSet); {
	case apierrors.IsNotFound(err):
		logger.Info("ClusterSyncSet does not exist; will need to create")
		needToCreate = true
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get ClusterSyncSet")
		return reconcile.Result{}, err
	}

	origStatus := clusterSyncSet.Status.DeepCopy()

	timeUntilFullReapply := r.timeUntilFullReapply(clusterSyncSet)
	needToDoFullReapply := timeUntilFullReapply <= 0
	if needToDoFullReapply {
		logger.WithField("timeUntilFullReapply", timeUntilFullReapply).Info("need to reapply all syncsets")
	}

	syncSets, err := r.getSyncSetsForClusterDeployment(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	selectorSyncSets, err := r.getSelectorSyncSetsForClusterDeployment(cd, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	var allErrs []error

	// Apply SyncSets
	syncSetsStatuses, err := r.applySyncSets(
		"SyncSet",
		syncSets,
		clusterSyncSet.Status.SyncSets,
		needToDoFullReapply,
		applier,
		logger,
	)
	if err != nil {
		allErrs = append(allErrs, err)
	}
	clusterSyncSet.Status.SyncSets = syncSetsStatuses

	// Apply SelectorSyncSets
	selectorSyncSetsStatuses, err := r.applySyncSets(
		"SelectorSyncSet",
		selectorSyncSets,
		clusterSyncSet.Status.SelectorSyncSets,
		needToDoFullReapply,
		applier,
		logger,
	)
	if err != nil {
		allErrs = append(allErrs, err)
	}
	clusterSyncSet.Status.SelectorSyncSets = selectorSyncSetsStatuses

	// Create or Update the ClusterSyncSet
	if needToDoFullReapply {
		clusterSyncSet.Status.LastFullApplyTime = metav1.Now()
	}

	setFailedCondition(clusterSyncSet, needToDoFullReapply, utilerrors.NewAggregate(allErrs))

	if needToCreate {
		logger.Info("creating ClusterSyncSet")
		clusterSyncSet.Namespace = cd.Namespace
		clusterSyncSet.Name = cd.Name
		clusterSyncSet.OwnerReferences = []metav1.OwnerReference{{
			APIVersion:         hivev1.SchemeGroupVersion.String(),
			Kind:               "ClusterDeployment",
			Name:               cd.Name,
			UID:                cd.UID,
			BlockOwnerDeletion: pointer.BoolPtr(true),
		}}
		if err := r.Create(context.Background(), clusterSyncSet); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create ClusterSyncSet")
			return reconcile.Result{}, err
		}
	} else if !reflect.DeepEqual(origStatus, &clusterSyncSet.Status) {
		logger.Info("updating ClusterSyncSet")
		if err := r.Status().Update(context.Background(), clusterSyncSet); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterSyncSet")
			return reconcile.Result{}, err
		}
	}

	if waitTime := r.timeUntilFullReapply(clusterSyncSet); waitTime > 0 {
		return reconcile.Result{RequeueAfter: waitTime}, nil
	}
	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileClusterSyncSet) applySyncSets(
	syncSetType string,
	syncSets []CommonSyncSet,
	syncSetStatuses []hivev1.MatchedSyncSetStatus,
	needToDoFullReapply bool,
	applier applier,
	logger log.FieldLogger,
) ([]hivev1.MatchedSyncSetStatus, error) {
	// Sort the syncsets to a consistent ordering. This prevents thrashing in the ClusterSyncSet status due to the order
	// of the syncset status changing from one reconcile to the next.
	sort.Slice(syncSets, func(i, j int) bool {
		return syncSets[i].AsMetaObject().GetName() < syncSets[j].AsMetaObject().GetName()
	})

	var allErrs []error
	var newStatuses []hivev1.MatchedSyncSetStatus

	for _, syncSet := range syncSets {
		logger := logger.WithField(syncSetType, syncSet.AsMetaObject().GetName())
		oldStatus, indexOfOldStatus := getOldSyncSetStatus(syncSet, syncSetStatuses)
		// Remove the matching old status from the slice of statuses so that the slice only contains statuses that have
		// not been matched to a syncset.
		if indexOfOldStatus >= 0 {
			last := len(syncSetStatuses) - 1
			syncSetStatuses[indexOfOldStatus] = syncSetStatuses[last]
			syncSetStatuses = syncSetStatuses[:last]
		}
		switch {
		case needToDoFullReapply:
			logger.Debug("applying syncset because it is time to do a full re-apply")
		case indexOfOldStatus < 0:
			logger.Debug("applying syncset because the syncset is new")
		case oldStatus.Result != hivev1.SuccessSyncSetResult:
			logger.Debug("applying syncset because the last attempt to apply failed")
		case oldStatus.ObservedGeneration != syncSet.GetStatus().ObservedGeneration:
			logger.Debug("applying syncset because the syncset generation has changed")
		default:
			logger.Debug("skipping apply of syncset since it is up-to-date and it is not time to do a full re-apply")
			newStatuses = append(newStatuses, oldStatus)
			continue
		}
		newStatus, applyErr := r.applySyncSet(syncSet, applier, logger)
		if applyErr == nil {
			logger.Info("syncset applied")
		} else {
			allErrs = append(allErrs, errors.Wrapf(applyErr, "failed to apply %s %s", syncSetType, syncSet.AsMetaObject().GetName()))
		}
		if indexOfOldStatus >= 0 {
			remainingResources, deleteErr := deleteResources(
				oldStatus.ResourcesToDelete,
				func(r hivev1.ResourceIdentification) bool { return !isResourceStillIncludedInSyncSet(r, syncSet) },
				applier,
				logger,
			)
			if deleteErr != nil {
				allErrs = append(allErrs, errors.Wrapf(deleteErr, "failed to delete resources for %s %s", syncSetType, syncSet.AsMetaObject().GetName()))
			}
			newStatus.ResourcesToDelete = mergeResources(newStatus.ResourcesToDelete, remainingResources)
		}
		// Sort ResourcesToDelete to prevent update thrashing.
		sort.Slice(newStatus.ResourcesToDelete, func(i, j int) bool {
			return orderResources(newStatus.ResourcesToDelete[i], newStatus.ResourcesToDelete[j])
		})
		newStatuses = append(newStatuses, newStatus)
	}

	// The remaining statuses in syncSetStatuses do not match any syncsets. Any resources to delete in the status need
	// to be deleted.
	for _, status := range syncSetStatuses {
		remainingResources, deleteErr := deleteResources(status.ResourcesToDelete, nil, applier, logger)
		if deleteErr != nil {
			allErrs = append(allErrs, errors.Wrapf(deleteErr, "failed to delete resources for %s %s", syncSetType, status.Name))
			newStatuses = append(newStatuses, hivev1.MatchedSyncSetStatus{
				Name:              status.Name,
				ResourcesToDelete: remainingResources,
				Result:            hivev1.FailureSyncSetResult,
			})
		}
	}

	return newStatuses, utilerrors.NewAggregate(allErrs)
}

func getOldSyncSetStatus(syncSet CommonSyncSet, syncSetStatuses []hivev1.MatchedSyncSetStatus) (hivev1.MatchedSyncSetStatus, int) {
	for i, status := range syncSetStatuses {
		if status.Name == syncSet.AsMetaObject().GetName() {
			return syncSetStatuses[i], i
		}
	}
	return hivev1.MatchedSyncSetStatus{}, -1
}

func (r *ReconcileClusterSyncSet) applySyncSet(syncSet CommonSyncSet, applier applier, logger log.FieldLogger) (hivev1.MatchedSyncSetStatus, error) {
	status := hivev1.MatchedSyncSetStatus{
		Name:               syncSet.AsMetaObject().GetName(),
		ObservedGeneration: syncSet.GetStatus().ObservedGeneration,
	}
	if syncSet.AsMetaObject().GetGeneration() != syncSet.GetStatus().ObservedGeneration {
		logger.Info("not applying syncset since the observed generation of the syncset does not match the generation of the syncset")
		status.Result = hivev1.FailureSyncSetResult
		return status, nil
	}
	for _, cond := range syncSet.GetStatus().Conditions {
		if cond.Type == hivev1.SyncSetInvalidResourceCondition &&
			cond.Status == corev1.ConditionTrue {
			logger.Info("not applying syncset as it contains an invalid resource")
			status.Result = hivev1.FailureSyncSetResult
			return status, nil
		}
	}
	applyFn := applier.Apply
	applyFnMetricsLabel := labelApply
	switch syncSet.GetSpec().ApplyBehavior {
	case hivev1.CreateOrUpdateSyncSetApplyBehavior:
		applyFn = applier.CreateOrUpdate
		applyFnMetricsLabel = labelCreateOrUpdate
	case hivev1.CreateOnlySyncSetApplyBehavior:
		applyFn = applier.Create
		applyFnMetricsLabel = labelCreateOnly
	}
	// Apply Resources
	for i, resource := range syncSet.GetSpec().Resources {
		identification := syncSet.GetStatus().Resources[i]
		logger := logger.WithField("resourceNamespace", identification.Namespace).
			WithField("resourceName", identification.Name).
			WithField("resourceAPIVersion", identification.APIVersion).
			WithField("resourceKind", identification.Kind)
		logger.Debug("applying resource")
		u := &unstructured.Unstructured{}
		if err := yaml.Unmarshal(resource.Raw, u); err != nil {
			logger.WithError(err).Warn("error decoding unstructured object")
			status.Result = hivev1.FailureSyncSetResult
			return status, errors.Wrapf(err, "failed to decode resource %d", i)
		}
		if err := applyResource(u, applyFnMetricsLabel, applyFn, logger); err != nil {
			status.Result = hivev1.FailureSyncSetResult
			return status, errors.Wrapf(err, "failed to apply resource %d", i)
		}
		if syncSet.GetSpec().ResourceApplyMode == hivev1.SyncResourceApplyMode {
			status.ResourcesToDelete = append(status.ResourcesToDelete, identification)
		}
	}
	// Apply Secrets
	for i, secretMapping := range syncSet.GetSpec().Secrets {
		logger := logger.WithField("secretNamespace", secretMapping.TargetRef.Namespace).
			WithField("secretName", secretMapping.TargetRef.Name)
		logger.Debug("applying secret")
		secret := &corev1.Secret{}
		if err := r.Get(context.Background(), types.NamespacedName{Namespace: syncSet.AsMetaObject().GetNamespace(), Name: secretMapping.SourceRef.Name}, secret); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "cannot read secret")
			status.Result = hivev1.FailureSyncSetResult
			return status, errors.Wrapf(err, "failed to read secret %d", i)
		}
		secret.Namespace = secretMapping.TargetRef.Namespace
		secret.Name = secretMapping.TargetRef.Name
		// These pieces of metadata need to be set to nil values to perform an update from the original secret
		secret.Generation = 0
		secret.ResourceVersion = ""
		secret.UID = ""
		secret.OwnerReferences = nil
		if err := applyResource(secret, applyFnMetricsLabel, applyFn, logger); err != nil {
			status.Result = hivev1.FailureSyncSetResult
			return status, errors.Wrapf(err, "failed to apply secret %d", i)
		}
		if syncSet.GetSpec().ResourceApplyMode == hivev1.SyncResourceApplyMode {
			status.ResourcesToDelete = append(status.ResourcesToDelete,
				hivev1.ResourceIdentification{
					APIVersion: secretAPIVersion,
					Kind:       secretKind,
					Namespace:  secretMapping.TargetRef.Namespace,
					Name:       secretMapping.TargetRef.Name,
				})
		}
	}
	// Apply Patches
	for i, patch := range syncSet.GetSpec().Patches {
		logger := logger.WithField("patchNamespace", patch.Namespace).
			WithField("patchName", patch.Name).
			WithField("patchAPIVersion", patch.APIVersion).
			WithField("patchKind", patch.Kind)
		logger.Debug("applying patch")
		if err := applier.Patch(
			types.NamespacedName{Namespace: patch.Namespace, Name: patch.Name},
			patch.Kind,
			patch.APIVersion,
			[]byte(patch.Patch),
			patch.PatchType,
		); err != nil {
			status.Result = hivev1.FailureSyncSetResult
			return status, errors.Wrapf(err, "failed to apply patch %d", i)
		}
	}
	status.Result = hivev1.SuccessSyncSetResult
	return status, nil
}

func applyResource(
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

func deleteResources(
	resources []hivev1.ResourceIdentification,
	shouldDelete func(hivev1.ResourceIdentification) bool,
	applier applier,
	logger log.FieldLogger,
) (remainingResources []hivev1.ResourceIdentification, returnErr error) {
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
		if err := applier.Delete(r.APIVersion, r.Kind, r.Namespace, r.Name); err != nil {
			logger.WithError(err).Warn("could not delete resource")
			allErrs = append(allErrs, err)
			remainingResources = append(remainingResources, r)
		}
	}
	return remainingResources, utilerrors.NewAggregate(allErrs)
}

func isResourceStillIncludedInSyncSet(resource hivev1.ResourceIdentification, syncSet CommonSyncSet) bool {
	if containsResource(syncSet.GetStatus().Resources, resource) {
		return true
	}
	if resource.APIVersion == secretAPIVersion && resource.Kind == secretKind {
		for _, s := range syncSet.GetSpec().Secrets {
			if s.TargetRef.Namespace == resource.Namespace &&
				s.TargetRef.Name == resource.Name {
				return true
			}
		}
	}
	return false
}

func (r *ReconcileClusterSyncSet) getSyncSetsForClusterDeployment(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]CommonSyncSet, error) {
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

func (r *ReconcileClusterSyncSet) getSelectorSyncSetsForClusterDeployment(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]CommonSyncSet, error) {
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

func setFailedCondition(clusterSyncSet *hivev1.ClusterSyncSet, fullApply bool, failureError error) {
	status := corev1.ConditionFalse
	reason := "Success"
	message := "All SyncSets and SelectorSyncSets have been applied to the cluster"
	if failureError != nil {
		status = corev1.ConditionTrue
		reason = "Failure"
		message = failureError.Error()
	}
	if !fullApply && len(clusterSyncSet.Status.Conditions) > 0 {
		cond := clusterSyncSet.Status.Conditions[0]
		if status == cond.Status &&
			reason == cond.Reason &&
			message == cond.Message {
			return
		}
	}
	clusterSyncSet.Status.Conditions = []hivev1.ClusterSyncSetCondition{{
		Type:               hivev1.ClusterSyncSetFailed,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}}
}

func mergeResources(a, b []hivev1.ResourceIdentification) []hivev1.ResourceIdentification {
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

func containsResource(resources []hivev1.ResourceIdentification, resource hivev1.ResourceIdentification) bool {
	for _, r := range resources {
		if r == resource {
			return true
		}
	}
	return false
}

func orderResources(a, b hivev1.ResourceIdentification) bool {
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

func (r *ReconcileClusterSyncSet) timeUntilFullReapply(clusterSyncSet *hivev1.ClusterSyncSet) time.Duration {
	return r.reapplyInterval - time.Since(clusterSyncSet.Status.LastFullApplyTime.Time) +
		time.Duration(0.1*rand.Float64()*r.reapplyInterval.Seconds())*time.Second
}

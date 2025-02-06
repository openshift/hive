package clusterpool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"

	"github.com/openshift/installer/pkg/types/nutanix"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/davegardnerisme/deephash"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/clusterresource"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	yamlpatch "github.com/openshift/hive/pkg/util/yaml"
)

const (
	ControllerName                  = hivev1.ClusterpoolControllerName
	finalizer                       = "hive.openshift.io/clusters"
	clusterPoolNamespaceLabelKey    = "hive.openshift.io/clusterpool-namespace"
	clusterPoolNameLabelKey         = "hive.openshift.io/clusterpool-name"
	imageSetDependent               = "cluster image set"
	pullSecretDependent             = "pull secret"
	credentialsSecretDependent      = "credentials secret"
	clusterPoolAdminRoleName        = "hive-cluster-pool-admin"
	clusterPoolAdminRoleBindingName = "hive-cluster-pool-admin-binding"
	icSecretDependent               = "install config template secret"
	cdClusterPoolIndex              = "spec.clusterpool.namespacedname"
	claimClusterPoolIndex           = "spec.clusterpoolname"
)

var (
	// clusterPoolConditions are the cluster pool conditions controlled or initialized by cluster pool controller
	clusterPoolConditions = []hivev1.ClusterPoolConditionType{
		hivev1.ClusterPoolMissingDependenciesCondition,
		hivev1.ClusterPoolCapacityAvailableCondition,
		hivev1.ClusterPoolAllClustersCurrentCondition,
		hivev1.ClusterPoolInventoryValidCondition,
		hivev1.ClusterPoolDeletionPossibleCondition,
	}
)

func poolKey(namespace, name string) string {
	return namespace + "/" + name
}

// Add creates a new ClusterPool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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

// NewReconciler returns a new ReconcileClusterPool
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *ReconcileClusterPool {
	logger := log.WithField("controller", ControllerName)
	return &ReconcileClusterPool{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		logger:       logger,
		expectations: controllerutils.NewExpectations(logger),
	}
}

func indexClusterDeploymentsByClusterPool(o client.Object) []string {
	cd := o.(*hivev1.ClusterDeployment)
	if poolRef := cd.Spec.ClusterPoolRef; poolRef != nil {
		return []string{poolKey(poolRef.Namespace, poolRef.PoolName)}
	}
	return []string{}
}

func indexClusterClaimsByClusterPool(o client.Object) []string {
	claim := o.(*hivev1.ClusterClaim)
	if poolName := claim.Spec.ClusterPoolName; poolName != "" {
		return []string{poolName}
	}
	return []string{}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterPool, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error {
	// Create a new controller
	c, err := controller.New("clusterpool-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, r.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Index ClusterDeployments by pool namespace/name
	if err := mgr.GetFieldIndexer().
		IndexField(
			context.TODO(), &hivev1.ClusterDeployment{}, cdClusterPoolIndex, indexClusterDeploymentsByClusterPool); err != nil {
		log.WithError(err).Error("Error indexing ClusterDeployments by ClusterPool")
		return err
	}

	// Index ClusterClaims by pool name
	if err := mgr.GetFieldIndexer().
		IndexField(
			context.TODO(), &hivev1.ClusterClaim{}, claimClusterPoolIndex, indexClusterClaimsByClusterPool); err != nil {
		log.WithError(err).Error("Error indexing ClusterClaims by ClusterPool")
		return err
	}

	// Watch for changes to ClusterPool
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterPool{}, &handler.TypedEnqueueRequestForObject[*hivev1.ClusterPool]{}))
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployments originating from a pool:
	if err := r.watchClusterDeployments(mgr, c); err != nil {
		return err
	}

	// Watch for changes to ClusterClaims
	enqueuePoolForClaim := handler.TypedEnqueueRequestsFromMapFunc(
		func(ctx context.Context, claim *hivev1.ClusterClaim) []reconcile.Request {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: claim.Namespace,
					Name:      claim.Spec.ClusterPoolName,
				},
			}}
		},
	)
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterClaim{}, enqueuePoolForClaim)); err != nil {
		return err
	}

	// Watch for changes to the hive cluster pool admin RoleBindings
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &rbacv1.RoleBinding{}, handler.TypedEnqueueRequestsFromMapFunc(
			requestsForRBACResources(r.Client, r.logger)),
		)); err != nil {
		return err
	}

	// Watch for changes to the hive-cluster-pool-admin-binding RoleBinding
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &rbacv1.RoleBinding{}, handler.TypedEnqueueRequestsFromMapFunc(
			requestsForCDRBACResources(r.Client, clusterPoolAdminRoleBindingName, r.logger)),
		)); err != nil {
		return err
	}

	// Watch for changes to ClusterDeploymentCustomizations
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hivev1.ClusterDeploymentCustomization{}, handler.TypedEnqueueRequestsFromMapFunc(
			requestsForCDCResources(r.Client, r.logger)),
		)); err != nil {
		return err
	}

	return nil
}

func requestsForCDCResources(c client.Client, logger log.FieldLogger) handler.TypedMapFunc[*hivev1.ClusterDeploymentCustomization, reconcile.Request] {
	return func(ctx context.Context, cdc *hivev1.ClusterDeploymentCustomization) []reconcile.Request {
		cpList := &hivev1.ClusterPoolList{}
		if err := c.List(context.Background(), cpList, client.InNamespace(cdc.GetNamespace())); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to list cluster pools for CDC resource")
			return nil
		}

		var requests []reconcile.Request
		for _, cpl := range cpList.Items {
			if cpl.Spec.Inventory == nil {
				continue
			}
			for _, entry := range cpl.Spec.Inventory {
				if entry.Name != cdc.Name {
					continue
				}
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Namespace: cpl.Namespace,
						Name:      cpl.Name,
					},
				})
				break
			}
		}

		return requests
	}
}

func requestsForCDRBACResources(c client.Client, resourceName string, logger log.FieldLogger) handler.TypedMapFunc[*rbacv1.RoleBinding, reconcile.Request] {
	return func(ctx context.Context, o *rbacv1.RoleBinding) []reconcile.Request {
		if o.GetName() != resourceName {
			return nil
		}
		clusterName := o.GetNamespace()
		cd := &hivev1.ClusterDeployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get ClusterDeployment for RBAC resource")
			return nil
		}
		cpKey := clusterPoolKey(cd)
		if cpKey == nil {
			return nil
		}
		return []reconcile.Request{{NamespacedName: *cpKey}}
	}
}

func requestsForRBACResources(c client.Client, logger log.FieldLogger) handler.TypedMapFunc[*rbacv1.RoleBinding, reconcile.Request] {
	return func(ctx context.Context, binding *rbacv1.RoleBinding) []reconcile.Request {
		if binding.RoleRef.Kind != "ClusterRole" || binding.RoleRef.Name != clusterPoolAdminRoleName {
			return nil
		}

		cpList := &hivev1.ClusterPoolList{}
		if err := c.List(context.Background(), cpList, client.InNamespace(binding.GetNamespace())); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to list cluster pools for RBAC resource")
			return nil
		}
		var requests []reconcile.Request
		for _, cpl := range cpList.Items {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: cpl.Namespace,
					Name:      cpl.Name,
				},
			})
		}
		return requests
	}
}

var _ reconcile.Reconciler = &ReconcileClusterPool{}

// ReconcileClusterPool reconciles a ClusterPool object
type ReconcileClusterPool struct {
	client.Client
	logger log.FieldLogger
	// A TTLCache of ClusterDeployment creates each ClusterPool expects to see
	expectations controllerutils.ExpectationsInterface
}

// Reconcile reads the state of the ClusterPool, checks if we currently have enough ClusterDeployments waiting, and
// attempts to reach the desired state if not.
func (r *ReconcileClusterPool) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterPool", request.NamespacedName)
	logger.Info("reconciling cluster pool")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterPool instance
	clp := &hivev1.ClusterPool{}
	err := r.Get(context.TODO(), request.NamespacedName, clp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("pool not found")
			r.expectations.DeleteExpectations(request.NamespacedName.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.WithError(err).Error("error reading cluster pool")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: clp}, logger)

	if clp.Spec.RunningCount != clp.Spec.Size && poolAlwaysRunning(clp) {
		return reconcile.Result{}, errors.New("Hibernation is not supported on Openstack, VShpere and Ovirt. Must set runningCount==size.")
	}

	// Initialize cluster pool conditions if not set
	newConditions, changed := controllerutils.InitializeClusterPoolConditions(clp.Status.Conditions, clusterPoolConditions)
	if changed {
		clp.Status.Conditions = newConditions
		logger.Info("initializing cluster pool conditions")
		if err := r.Status().Update(context.TODO(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster pool status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// If the pool is deleted, clear finalizer once all ClusterDeployments have been deleted.
	if clp.DeletionTimestamp != nil {
		return reconcile.Result{}, r.reconcileDeletedPool(clp, logger)
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(clp, finalizer) {
		logger.Debug("adding finalizer to ClusterPool")
		controllerutils.AddFinalizer(clp, finalizer)
		if err := r.Update(context.Background(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer to ClusterPool")
			return reconcile.Result{}, err
		}
	}

	if !r.expectations.SatisfiedExpectations(request.NamespacedName.String()) {
		logger.Debug("waiting for expectations to be satisfied")
		return reconcile.Result{}, nil
	}

	poolVersion := calculatePoolVersion(clp)

	cds, err := getAllClusterDeploymentsForPool(r.Client, clp, poolVersion, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	claims, err := getAllClaimsForPool(r.Client, clp, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	cdcs, err := getAllCustomizationsForPool(r.Client, clp, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	claims.SyncClusterDeploymentAssignments(r.Client, cds, logger)
	cds.SyncClaimAssignments(r.Client, claims, logger)
	if err := cdcs.SyncClusterDeploymentCustomizationAssignments(r.Client, clp, cds, logger); err != nil {
		return reconcile.Result{}, err
	}

	if setStatusCounts(clp, cds) {
		if err := r.Status().Update(context.Background(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterPool status")
			return reconcile.Result{}, errors.Wrap(err, "could not update ClusterPool status")
		}
	}

	availableCapacity := math.MaxInt32
	if clp.Spec.MaxSize != nil {
		availableCapacity = int(*clp.Spec.MaxSize) - cds.Total()
		numAssigned := cds.NumAssigned()
		if availableCapacity <= 0 {
			logger.WithFields(log.Fields{
				"UnclaimedSize": len(cds.Unassigned(true)),
				"ClaimedSize":   numAssigned,
				"Capacity":      *clp.Spec.MaxSize,
			}).Info("Cannot add more clusters because no capacity available.")
		}
	}
	if err := r.setAvailableCapacityCondition(clp, availableCapacity > 0, logger); err != nil {
		logger.WithError(err).Error("error setting CapacityAvailable condition")
		return reconcile.Result{}, err
	}

	if err := assignClustersToClaims(r.Client, claims, cds, logger); err != nil {
		logger.WithError(err).Error("error assigning clusters <=> claims")
		return reconcile.Result{}, err
	}

	availableCurrent := math.MaxInt32
	if clp.Spec.MaxConcurrent != nil {
		availableCurrent = int(*clp.Spec.MaxConcurrent) - len(cds.Installing()) - len(cds.Deleting())
		if availableCurrent < 0 {
			availableCurrent = 0
		}
	}

	// remove clusters that were previously claimed but now not required.
	toRemoveClaimedCDs := cds.MarkedForDeletion()
	toDel := minIntVarible(len(toRemoveClaimedCDs), availableCurrent)
	for _, cd := range toRemoveClaimedCDs[:toDel] {
		cdLog := logger.WithField("cluster", cd.Name)
		cdLog.Info("deleting cluster deployment for previous claim")
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			cdLog.WithError(err).Error("error deleting cluster deployment")
			return reconcile.Result{}, err
		}
	}
	availableCurrent -= toDel

	// drift will indicate how many clusters we need to add or delete to get back to steady state
	// of the pool's Size. This needs to take into account the clusters we're creating to satisfy
	// the immediate demand of pending claims.
	switch drift := len(cds.Unassigned(true)) - int(clp.Spec.Size) - len(claims.Unassigned()); {
	// activity quota exceeded, so no action
	case availableCurrent <= 0:
		logger.WithFields(log.Fields{
			"MaxConcurrent": *clp.Spec.MaxConcurrent,
			"Available":     availableCurrent,
		}).Info("Cannot create/delete clusters as max concurrent quota exceeded.")
	// If too few, create new InstallConfig and ClusterDeployment.
	case drift < 0 && availableCapacity > 0:
		toAdd := minIntVarible(-drift, availableCapacity, availableCurrent)
		if clp.Spec.Inventory != nil {
			toAdd = minIntVarible(toAdd, len(cdcs.Unassigned()))
		}
		if err := r.addClusters(clp, poolVersion, cds, toAdd, cdcs, logger); err != nil {
			log.WithError(err).Error("error adding clusters")
			return reconcile.Result{}, err
		}
	// Delete broken CDs. We put this case after the "addClusters" case because we would rather
	// consume our maxConcurrent with additions than deletions. But we put it before the
	// "deleteExcessClusters" case because we would rather trim broken clusters than viable ones.
	case len(cds.Broken()) > 0:
		if err := r.deleteBrokenClusters(cds, availableCurrent, logger); err != nil {
			return reconcile.Result{}, err
		}
	// If too many, delete some.
	case drift > 0:
		toDel := minIntVarible(drift, availableCurrent)
		if err := r.deleteExcessClusters(cds, toDel, logger); err != nil {
			return reconcile.Result{}, err
		}
	// Special case for stale CDs: allow deleting one if all CDs are installed.
	case drift == 0 && len(cds.Installing()) == 0 && len(cds.Stale()) > 0:
		toDelete := cds.Stale()[0]
		logger := logger.WithField("cluster", toDelete.Name)
		logger.Info("deleting cluster deployment")
		if err := cds.Delete(r.Client, toDelete.Name); err != nil {
			logger.WithError(err).Error("error deleting cluster deployment")
			return reconcile.Result{}, err
		}
		metricStaleClusterDeploymentsDeleted.WithLabelValues(clp.Namespace, clp.Name).Inc()
	}

	if err := r.reconcileRunningClusters(clp, cds, len(claims.Unassigned()), logger); err != nil {
		log.WithError(err).Error("error updating hibernating/running state")
		return reconcile.Result{}, err
	}

	// One more (possible) status update: wait until the end to detect whether all unassigned CDs
	// are current with the pool config, since assigning or deleting CDs may eliminate the last
	// one that isn't.
	if err := setCDsCurrentCondition(r.Client, cds, clp); err != nil {
		log.WithError(err).Error("error updating 'ClusterDeployments current' status")
		return reconcile.Result{}, err
	}

	if err := r.reconcileRBAC(clp, logger); err != nil {
		log.WithError(err).Error("error reconciling RBAC")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// reconcileRunningClusters ensures the oldest unassigned clusters are set to running, and the
// remainder are set to hibernating. The number of clusters we set to running is determined by
// adding the cluster's configured runningCount to the number of unsatisfied claims for which we're
// spinning up new clusters.
func (r *ReconcileClusterPool) reconcileRunningClusters(
	clp *hivev1.ClusterPool,
	cds *cdCollection,
	extraRunning int,
	logger log.FieldLogger,
) error {
	// If we're creating excess clusters to satisfy unassigned claims, add that many
	// to the runningCount. They'll get snatched up immediately, bringing the number
	// of running clusters back down to runningCount once the pool reaches steady state.
	runningCount := int(clp.Spec.RunningCount) + extraRunning
	// Exclude broken clusters
	cdList := cds.Unassigned(false)
	// Sort by age, oldest first, for FIFO purposes. Include secondary sort by namespace/name as
	// a tie breaker in case creationTimestamps conflict (which they can, as they have a
	// granularity of 1s).
	sort.Slice(
		cdList,
		func(i, j int) bool {
			if !cdList[i].CreationTimestamp.Equal(&cdList[j].CreationTimestamp) {
				return cdList[i].CreationTimestamp.Before(&cdList[j].CreationTimestamp)
			}
			return cdList[i].Namespace < cdList[j].Namespace
		},
	)
	for i := 0; i < len(cdList); i++ {
		cd := cdList[i]
		var desiredPowerState hivev1.ClusterPowerState
		if i < runningCount {
			desiredPowerState = hivev1.ClusterPowerStateRunning
		} else {
			desiredPowerState = hivev1.ClusterPowerStateHibernating
		}
		if cd.Spec.PowerState == desiredPowerState {
			continue
		}
		cd.Spec.PowerState = desiredPowerState
		contextLogger := logger.WithFields(log.Fields{
			"cluster":       cd.Name,
			"newPowerState": desiredPowerState,
		})
		contextLogger.Info("Changing cluster powerState to satisfy runningCount")
		if err := r.Update(context.Background(), cd); err != nil {
			contextLogger.WithError(err).Error("could not update powerState")
			return err
		}
	}
	return nil
}

// calculatePoolVersion computes a hash of the important (to ClusterDeployments) fields of the
// ClusterPool.Spec. This is annotated on ClusterDeployments when the pool creates them, which
// subsequently allows us to tell whether all unclaimed CDs are up to date and set a condition
// accordingly.
// NOTE: If we change this algorithm, we're guaranteed to think that all CDs are stale at the
// moment that code update rolls out. We may wish to consider a way to support detecting the old
// value as "current" in that case.
func calculatePoolVersion(clp *hivev1.ClusterPool) string {
	ba := []byte{}
	ba = append(ba, deephash.Hash(clp.Spec.Platform)...)
	ba = append(ba, deephash.Hash(clp.Spec.BaseDomain)...)
	ba = append(ba, deephash.Hash(clp.Spec.ImageSetRef)...)
	ba = append(ba, deephash.Hash(clp.Spec.InstallConfigSecretTemplateRef)...)
	// Inventory changes the behavior of cluster pool, thus it needs to be in the pool version.
	// But to avoid redployment of clusters if inventory changes, a fixed string is added to pool version.
	// https://github.com/openshift/hive/blob/master/docs/enhancements/clusterpool-inventory.md#pool-version
	if clp.Spec.Inventory != nil {
		ba = append(ba, []byte("hasInventory")...)
	}
	// Hash of hashes to ensure fixed length
	return fmt.Sprintf("%x", deephash.Hash(ba))
}

func minIntVarible(v1 int, vn ...int) (m int) {
	m = v1
	for i := 0; i < len(vn); i++ {
		if vn[i] < m {
			m = vn[i]
		}
	}
	return
}

func (r *ReconcileClusterPool) reconcileRBAC(
	clp *hivev1.ClusterPool,
	logger log.FieldLogger,
) error {
	rbList := &rbacv1.RoleBindingList{}
	if err := r.Client.List(context.Background(), rbList, client.InNamespace(clp.GetNamespace())); err != nil {
		log.WithError(err).Error("error listing rolebindings")
		return err
	}
	var subs []rbacv1.Subject
	var roleRef rbacv1.RoleRef
	for _, rb := range rbList.Items {
		if rb.RoleRef.Kind != "ClusterRole" ||
			rb.RoleRef.Name != clusterPoolAdminRoleName {
			continue
		}
		roleRef = rb.RoleRef
		subs = append(subs, rb.Subjects...)
	}

	if len(subs) == 0 {
		// nothing to do here.
		return nil
	}

	// get namespaces where role needs to be bound.
	cdNSList := &corev1.NamespaceList{}
	if err := r.Client.List(context.Background(), cdNSList,
		client.MatchingLabels{constants.ClusterPoolNameLabel: clp.GetName()}); err != nil {
		log.WithError(err).Error("error listing namespaces for cluster pool")
		return err
	}

	// create role bindings.
	var errs []error
	for _, ns := range cdNSList.Items {
		if ns.DeletionTimestamp != nil {
			logger.WithField("namespace", ns.GetName()).
				Debug("skipping syncing the rolebindings as the namespace is marked for deletion")
			continue
		}
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.GetName(),
				Name:      clusterPoolAdminRoleBindingName,
			},
			Subjects: subs,
			RoleRef:  roleRef,
		}
		orb := &rbacv1.RoleBinding{}
		if err := r.applyRoleBinding(rb, orb, logger); err != nil {
			errs = append(errs, errors.Wrapf(err, "failed to apply rolebinding in namespace %s", ns.GetNamespace()))
			continue
		}
	}
	if err := utilerrors.NewAggregate(errs); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterPool) applyRoleBinding(desired, observed *rbacv1.RoleBinding, logger log.FieldLogger) error {
	key := client.ObjectKey{Namespace: desired.GetNamespace(), Name: desired.GetName()}
	logger = logger.WithField("rolebinding", key)

	switch err := r.Client.Get(context.Background(), key, observed); {
	case apierrors.IsNotFound(err):
		logger.Info("creating rolebinding")
		if err := r.Create(context.Background(), desired); err != nil {
			logger.WithError(err).Error("could not create rolebinding")
			return err
		}
		return nil
	case err != nil:
		logger.WithError(err).Error("error getting rolebinding")
		return errors.Wrap(err, "could not get rolebinding")
	}

	if reflect.DeepEqual(observed.Subjects, desired.Subjects) && reflect.DeepEqual(observed.RoleRef, desired.RoleRef) {
		return nil
	}
	obsrvdSubs := observed.Subjects
	observed.Subjects = desired.Subjects
	observed.RoleRef = desired.RoleRef

	logger.WithFields(log.Fields{
		"observedSubjects": obsrvdSubs,
		"desiredSubjects":  desired.Subjects,
		"desiredRoleRef":   desired.RoleRef,
	}).Info("updating rolebinding")
	if err := r.Update(context.Background(), observed); err != nil {
		logger.WithError(err).Error("could not update rolebinding")
		return err
	}

	return nil
}

func (r *ReconcileClusterPool) addClusters(
	clp *hivev1.ClusterPool,
	poolVersion string,
	cds *cdCollection,
	newClusterCount int,
	cdcs *cdcCollection,
	logger log.FieldLogger,
) error {
	logger.WithField("count", newClusterCount).Info("Adding new clusters")

	var errs []error

	if err := r.verifyClusterImageSet(clp, logger); err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", imageSetDependent, err))
	}

	// Load the pull secret if one is specified (may not be if using a global pull secret)
	pullSecret, err := r.getPullSecret(clp, logger)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", pullSecretDependent, err))
	}

	// Load install config templates if specified
	installConfigTemplate, err := r.getInstallConfigTemplate(clp, logger)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", icSecretDependent, err))
	}

	cloudBuilder, err := r.createCloudBuilder(clp, logger)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", credentialsSecretDependent, err))
	}

	dependenciesError := utilerrors.NewAggregate(errs)

	if err := r.setMissingDependenciesCondition(clp, dependenciesError, logger); err != nil {
		return err
	}

	if dependenciesError != nil {
		return dependenciesError
	}

	for i := 0; i < newClusterCount; i++ {
		cd, err := r.createCluster(clp, cloudBuilder, pullSecret, installConfigTemplate, poolVersion, cdcs, logger)
		if err != nil {
			return err
		}
		cds.RegisterNewCluster(cd)
	}

	return nil
}

func (r *ReconcileClusterPool) createCluster(
	clp *hivev1.ClusterPool,
	cloudBuilder clusterresource.CloudBuilder,
	pullSecret string,
	installConfigTemplate string,
	poolVersion string,
	cdcs *cdcCollection,
	logger log.FieldLogger,
) (*hivev1.ClusterDeployment, error) {
	var err error

	ns, err := r.createRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return nil, err
	}
	logger.WithField("cluster", ns.Name).Info("Creating new cluster")

	annotations := clp.Spec.Annotations
	// Annotate the CD so we can distinguish "stale" CDs
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[constants.ClusterDeploymentPoolSpecHashAnnotation] = poolVersion

	labels := clp.Spec.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	// Add clusterpool ownership labels for easy searching
	labels[clusterPoolNamespaceLabelKey] = clp.Namespace
	labels[clusterPoolNameLabelKey] = clp.Name

	// We will use this unique random namespace name for our cluster name.
	builder := &clusterresource.Builder{
		Name:                  ns.Name,
		Namespace:             ns.Name,
		BaseDomain:            clp.Spec.BaseDomain,
		ImageSet:              clp.Spec.ImageSetRef.Name,
		WorkerNodesCount:      int64(3),
		MachineNetwork:        "10.0.0.0/16",
		PullSecret:            pullSecret,
		CloudBuilder:          cloudBuilder,
		Labels:                labels,
		Annotations:           annotations,
		InstallConfigTemplate: installConfigTemplate,
		InstallAttemptsLimit:  clp.Spec.InstallAttemptsLimit,
		InstallerEnv:          clp.Spec.InstallerEnv,
		SkipMachinePools:      clp.Spec.SkipMachinePools,
	}

	if clp.Spec.HibernateAfter != nil {
		builder.HibernateAfter = &clp.Spec.HibernateAfter.Duration
	}

	objs, err := builder.Build()
	if err != nil {
		return nil, errors.Wrap(err, "error building resources")
	}

	poolKey := types.NamespacedName{Namespace: clp.Namespace, Name: clp.Name}.String()
	r.expectations.ExpectCreations(poolKey, 1)
	var cd *hivev1.ClusterDeployment
	var secret *corev1.Secret
	var cdPos int
	// Add the ClusterPoolRef to the ClusterDeployment, and move it to the end of the slice.
	for i, obj := range objs {
		if cdTmp, ok := obj.(*hivev1.ClusterDeployment); ok {
			cd = cdTmp
			cdPos = i
			poolRef := poolReference(clp)
			cd.Spec.ClusterPoolRef = &poolRef
			if clp.Spec.Inventory != nil {
				cdc := cdcs.unassigned[0]
				if cdcs.reserved[cdc.Name] != nil || cdc.Status.ClusterDeploymentRef != nil || cdc.Status.ClusterPoolRef != nil {
					return nil, errors.Errorf("ClusterDeploymentCustomization %s is already reserved", cdc.Name)
				}
				cd.Spec.ClusterPoolRef.CustomizationRef = &corev1.LocalObjectReference{Name: cdc.Name}
			}
		} else if secretTmp := isInstallConfigSecret(obj); secretTmp != nil {
			secret = secretTmp
		}
	}

	if err := r.patchInstallConfig(clp, cd, secret, cdcs); err != nil {
		return nil, err
	}

	// Move the ClusterDeployment to the end of the slice
	lastIndex := len(objs) - 1
	objs[cdPos], objs[lastIndex] = objs[lastIndex], objs[cdPos]
	// Create the resources.
	for _, obj := range objs {
		if err := r.Client.Create(context.Background(), obj.(client.Object)); err != nil {
			r.expectations.CreationObserved(poolKey)
			return nil, err
		}
	}

	return cd, nil
}

func isInstallConfigSecret(obj interface{}) *corev1.Secret {
	if secret, ok := obj.(*corev1.Secret); ok {
		_, ok := secret.StringData["install-config.yaml"]
		if ok {
			return secret
		}
	}
	return nil
}

// patchInstallConfig responsible for applying ClusterDeploymentCustomization and its reservation
func (r *ReconcileClusterPool) patchInstallConfig(clp *hivev1.ClusterPool, cd *hivev1.ClusterDeployment, secret *corev1.Secret, cdcs *cdcCollection) error {
	if clp.Spec.Inventory == nil {
		return nil
	}
	if cd.Spec.ClusterPoolRef.CustomizationRef == nil {
		return errors.New("missing customization")
	}

	cdc := &hivev1.ClusterDeploymentCustomization{}
	if err := r.Client.Get(context.Background(), client.ObjectKey{Namespace: clp.Namespace, Name: cd.Spec.ClusterPoolRef.CustomizationRef.Name}, cdc); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.New("missing customization")
		}
		return err
	}

	installConfig, err := yamlpatch.ApplyPatches([]byte(secret.StringData["install-config.yaml"]), cdc.Spec.InstallConfigPatches)
	if err != nil {
		cdcs.BrokenBySyntax(r, cdc, fmt.Sprint(err))
		cdcs.UpdateInventoryValidCondition(r, clp)
		return err
	}

	configJson, err := json.Marshal(cdc.Spec)
	if err != nil {
		return err
	}

	if err := cdcs.Reserve(r, cdc, cd.Name, clp.Name); err != nil {
		return err
	}
	if err := cdcs.InstallationPending(r, cdc); err != nil {
		return err
	}

	cdc.Status.LastAppliedConfiguration = string(configJson)
	secret.StringData["install-config.yaml"] = string(installConfig)
	return nil
}

func (r *ReconcileClusterPool) createRandomNamespace(clp *hivev1.ClusterPool) (*corev1.Namespace, error) {
	namespaceName := apihelpers.GetResourceName(clp.Name, utilrand.String(5))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Labels: map[string]string{
				// Should never be removed.
				constants.ClusterPoolNameLabel: clp.Name,
			},
		},
	}
	err := r.Create(context.Background(), ns)
	return ns, err
}

// getClustersToDelete returns a list of length `deletionsNeeded` (to a max of the total number of
// unclaimed clusters) containing references to clusters that should be deleted. This method is
// used to reduce the size of the pool, usually in response to the user editing `size` and/or
// `maxSize`. The logic is such that we prioritize deleting clusters from the longest to the
// shortest amount of time before they're likely to be able to satisfy claims. That is:
// - We first queue up Installing clusters. They would need to finish provisioning.
// - Next, Standby clusters, which would need to be resumed.
// - Finally, Assignable clusters, which are already ready to be claimed.
func getClustersToDelete(cds *cdCollection, deletionsNeeded int, logger log.FieldLogger) []*hivev1.ClusterDeployment {
	installingClusters := cds.Installing()
	if deletionsNeeded <= len(installingClusters) {
		return installingClusters[:deletionsNeeded]
	}

	clustersToDelete := installingClusters
	origDeletionsNeeded := deletionsNeeded
	deletionsNeeded -= len(installingClusters)

	standbyClusters := cds.Standby()
	if deletionsNeeded <= len(standbyClusters) {
		return append(clustersToDelete, standbyClusters[:deletionsNeeded]...)
	}

	clustersToDelete = append(clustersToDelete, standbyClusters...)
	deletionsNeeded -= len(standbyClusters)

	readyClusters := cds.Assignable()
	if deletionsNeeded <= len(readyClusters) {
		return append(clustersToDelete, readyClusters[:deletionsNeeded]...)
	}

	logger.WithField("deletionsNeeded", origDeletionsNeeded).
		WithField("installingClusters", len(installingClusters)).
		WithField("standbyClusters", len(standbyClusters)).
		WithField("readyClusters", len(readyClusters)).
		Error("trying to delete more clusters than there are available")

	return append(clustersToDelete, readyClusters...)
}

func (r *ReconcileClusterPool) deleteExcessClusters(
	cds *cdCollection,
	deletionsNeeded int,
	logger log.FieldLogger,
) error {

	logger.WithField("deletionsNeeded", deletionsNeeded).Info("deleting excess clusters")
	clustersToDelete := getClustersToDelete(cds, deletionsNeeded, logger)
	for _, cd := range clustersToDelete {
		logger := logger.WithField("cluster", cd.Name)
		logger.Info("deleting cluster deployment")
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			logger.WithError(err).Error("error deleting cluster deployment")
			return err
		}
	}
	logger.Info("no more deletions required")
	return nil
}

func (r *ReconcileClusterPool) deleteBrokenClusters(cds *cdCollection, maxToDelete int, logger log.FieldLogger) error {
	numToDel := minIntVarible(maxToDelete, len(cds.Broken()))
	logger.WithField("numberToDelete", numToDel).Info("deleting broken clusters")
	clustersToDelete := make([]*hivev1.ClusterDeployment, numToDel)
	copy(clustersToDelete, cds.Broken()[:numToDel])
	for _, cd := range clustersToDelete {
		logger.WithField("cluster", cd.Name).Info("deleting broken cluster deployment")
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			logger.WithError(err).Error("error deleting cluster deployment")
			return err
		}
	}
	return nil
}

func (r *ReconcileClusterPool) reconcileDeletedPool(pool *hivev1.ClusterPool, logger log.FieldLogger) error {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return nil
	}

	cdcs, err := getAllCustomizationsForPool(r.Client, pool, logger)
	if err != nil {
		return err
	}

	if err := cdcs.RemoveFinalizer(r.Client, pool); err != nil {
		return err
	}

	// Don't care about the poolVersion here since we're deleting everything.
	cds, err := getAllClusterDeploymentsForPool(r.Client, pool, "", logger)
	if err != nil {
		return err
	}

	// Adhere to MaxConcurrent when cleaning up.
	availableConcurrent := math.MaxInt32
	if pool.Spec.MaxConcurrent != nil {
		availableConcurrent = int(*pool.Spec.MaxConcurrent) - len(cds.Installing()) - len(cds.Deleting())
	}

	// Clean up marked-for-deletion claimed clusters first. It really doesn't matter; the tie-breaker I'm using
	// is that claimed clusters are more likely to be running than those still in the pool (I'm not going to
	// bother checking runningCount) and are therefore more costly to keep running.
	for _, cd := range cds.MarkedForDeletion() {
		if availableConcurrent <= 0 {
			break
		}
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			logger.WithError(err).WithField("cluster", cd.Name).Log(controllerutils.LogLevel(err), "could not delete claimed ClusterDeployment")
			return errors.Wrap(err, "could not delete claimed ClusterDeployment")
		}
		availableConcurrent--
	}
	for _, cd := range cds.Unassigned(true) {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if availableConcurrent <= 0 {
			break
		}
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			logger.WithError(err).WithField("cluster", cd.Name).Log(controllerutils.LogLevel(err), "could not delete unassigned ClusterDeployment")
			return errors.Wrap(err, "could not delete unassigned ClusterDeployment")
		}
		availableConcurrent--
	}

	changedCond := setDeletionPossibleCondition(pool, cds)
	changedCounts := setStatusCounts(pool, cds)
	if changedCond || changedCounts {
		if err := r.Status().Update(context.Background(), pool); err != nil {
			return errors.Wrap(err, "could not update ClusterPool status")
		}
	}

	if cds.Total() != 0 {
		logger.Infof("Keeping ClusterPool alive, waiting for %d ClusterDeployment(s) to be deleted.", cds.Total())
		return nil
	}

	logger.Debug("All pool clusters deleted; removing finalizer")
	controllerutils.DeleteFinalizer(pool, finalizer)
	if err := r.Update(context.Background(), pool); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer from ClusterPool")
		return errors.Wrap(err, "could not delete finalizer from ClusterPool")
	}
	return nil
}

func poolReference(pool *hivev1.ClusterPool) hivev1.ClusterPoolReference {
	return hivev1.ClusterPoolReference{
		Namespace: pool.Namespace,
		PoolName:  pool.Name,
	}
}

// poolAlwaysRunning returns true if the Platrform, cloud provider, machines can only be in running state
func poolAlwaysRunning(pool *hivev1.ClusterPool) bool {
	p := pool.Spec.Platform
	return p.OpenStack != nil || p.Ovirt != nil || p.VSphere != nil
}

func (r *ReconcileClusterPool) getCredentialsSecret(pool *hivev1.ClusterPool, secretName string, logger log.FieldLogger) (*corev1.Secret, error) {
	credsSecret := &corev1.Secret{}
	if err := r.Client.Get(
		context.Background(),
		client.ObjectKey{Namespace: pool.Namespace, Name: secretName},
		credsSecret,
	); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error looking up credentials secret for pool in hive namespace")
		return nil, err
	}
	return credsSecret, nil
}

func (r *ReconcileClusterPool) setMissingDependenciesCondition(pool *hivev1.ClusterPool, err error, logger log.FieldLogger) error {
	status := corev1.ConditionFalse
	reason := "Verified"
	message := "Dependencies verified"
	updateConditionCheck := controllerutils.UpdateConditionNever
	if err != nil {
		status = corev1.ConditionTrue
		reason = "Missing"
		message = err.Error()
		updateConditionCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	conds, changed := controllerutils.SetClusterPoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.ClusterPoolMissingDependenciesCondition,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	if changed {
		pool.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), pool); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterPool conditions")
			return fmt.Errorf("could not update ClusterPool conditions: %w", err)
		}
	}
	return nil
}

func (r *ReconcileClusterPool) setAvailableCapacityCondition(pool *hivev1.ClusterPool, available bool, logger log.FieldLogger) error {
	status := corev1.ConditionTrue
	reason := "Available"
	message := "There is capacity to add more clusters to the pool."
	updateConditionCheck := controllerutils.UpdateConditionNever
	if !available {
		status = corev1.ConditionFalse
		reason = "MaxCapacity"
		message = fmt.Sprintf("Pool is at maximum capacity of %d waiting and claimed clusters.", *pool.Spec.MaxSize)
		updateConditionCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	conds, changed := controllerutils.SetClusterPoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.ClusterPoolCapacityAvailableCondition,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	if changed {
		pool.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), pool); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterPool conditions")
			return errors.Wrap(err, "could not update ClusterPool conditions")
		}
	}
	return nil
}

// setDeletionPossibleCondition updates the DeletionPossible condition on the pool and returns whether it
// changed. The caller is responsible for pushing the update to the server.
func setDeletionPossibleCondition(pool *hivev1.ClusterPool, cds *cdCollection) bool {
	status := corev1.ConditionTrue
	reason := "DeletionPossible"
	message := "No ClusterDeployments pending cleanup"
	updateConditionCheck := controllerutils.UpdateConditionNever
	if cds.Total() > 0 {
		status = corev1.ConditionFalse
		reason = "ClusterDeploymentsPendingCleanup"
		message = fmt.Sprintf("The following ClusterDeployments are pending cleanup: %s", strings.Join(cds.Names(), ", "))
		updateConditionCheck = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	conds, changed := controllerutils.SetClusterPoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.ClusterPoolDeletionPossibleCondition,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	pool.Status.Conditions = conds
	return changed
}

// setStatusCounts sets the Size, Standby, and Ready status fields in clp according to cds.
// The caller is responsible for pushing the changes back to the server.
// The return indicates whether anything changed.
func setStatusCounts(clp *hivev1.ClusterPool, cds *cdCollection) bool {
	origStatus := clp.Status.DeepCopy()
	clp.Status.Size = int32(len(cds.Unassigned(true)))
	clp.Status.Standby = int32(len(cds.Standby()))
	clp.Status.Ready = int32(len(cds.Assignable()))
	return !reflect.DeepEqual(origStatus, &clp.Status)
}

func (r *ReconcileClusterPool) verifyClusterImageSet(pool *hivev1.ClusterPool, logger log.FieldLogger) error {
	err := r.Get(context.Background(), client.ObjectKey{Name: pool.Spec.ImageSetRef.Name}, &hivev1.ClusterImageSet{})
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error getting cluster image set")
	}
	return err
}

func (r *ReconcileClusterPool) getInstallConfigTemplate(pool *hivev1.ClusterPool, logger log.FieldLogger) (string, error) {
	if pool.Spec.InstallConfigSecretTemplateRef == nil {
		return "", nil
	}
	installConfigSecret := &corev1.Secret{}
	err := r.Client.Get(
		context.Background(),
		types.NamespacedName{Namespace: pool.Namespace, Name: pool.Spec.InstallConfigSecretTemplateRef.Name},
		installConfigSecret,
	)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error reading install-config secret")
		return "", err
	}
	installConfig, ok := installConfigSecret.Data["install-config.yaml"]
	if !ok {
		logger.Info("install-config secret does not contain install-config.yaml data")
		return "", errors.New("install-config secret does not contain install-config.yaml data")
	}
	return string(installConfig), nil

}

func (r *ReconcileClusterPool) getPullSecret(pool *hivev1.ClusterPool, logger log.FieldLogger) (string, error) {
	if pool.Spec.PullSecretRef == nil {
		return "", nil
	}
	pullSecretSecret := &corev1.Secret{}
	err := r.Client.Get(
		context.Background(),
		types.NamespacedName{Namespace: pool.Namespace, Name: pool.Spec.PullSecretRef.Name},
		pullSecretSecret,
	)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error reading pull secret")
		return "", err
	}
	pullSecret, ok := pullSecretSecret.Data[".dockerconfigjson"]
	if !ok {
		logger.Info("pull secret does not contain .dockerconfigjson data")
		return "", errors.New("pull secret does not contain .dockerconfigjson data")
	}
	return string(pullSecret), nil
}

func (r *ReconcileClusterPool) createCloudBuilder(pool *hivev1.ClusterPool, logger log.FieldLogger) (clusterresource.CloudBuilder, error) {
	switch platform := pool.Spec.Platform; {
	case platform.AWS != nil:
		var cloudBuilder *clusterresource.AWSCloudBuilder
		switch {
		case platform.AWS.CredentialsSecretRef.Name != "":
			credsSecret, err := r.getCredentialsSecret(pool, platform.AWS.CredentialsSecretRef.Name, logger)
			if err != nil {
				return nil, err
			}
			cloudBuilder = clusterresource.NewAWSCloudBuilderFromSecret(credsSecret)
		case platform.AWS.CredentialsAssumeRole != nil:
			cloudBuilder = clusterresource.NewAWSCloudBuilderFromAssumeRole(platform.AWS.CredentialsAssumeRole)
		}

		cloudBuilder.Region = platform.AWS.Region
		// TODO: Plumb in an option for this
		cloudBuilder.InstanceType = clusterresource.AWSInstanceTypeDefault
		return cloudBuilder, nil
	case platform.GCP != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.GCP.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}
		cloudBuilder, err := clusterresource.NewGCPCloudBuilderFromSecret(credsSecret)
		if err != nil {
			logger.WithError(err).Info("could not build GCP cloud builder")
			return nil, err
		}
		// may be nil
		cloudBuilder.DiscardLocalSsdOnHibernate = platform.GCP.DiscardLocalSsdOnHibernate
		cloudBuilder.Region = platform.GCP.Region
		return cloudBuilder, nil
	case platform.Azure != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.Azure.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}
		cloudBuilder := clusterresource.NewAzureCloudBuilderFromSecret(credsSecret)
		cloudBuilder.BaseDomainResourceGroupName = platform.Azure.BaseDomainResourceGroupName
		cloudBuilder.Region = platform.Azure.Region
		cloudBuilder.CloudName = platform.Azure.CloudName
		return cloudBuilder, nil
	case platform.OpenStack != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.OpenStack.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}
		cloudBuilder := clusterresource.NewOpenStackCloudBuilderFromSecret(credsSecret)
		cloudBuilder.Cloud = platform.OpenStack.Cloud
		return cloudBuilder, nil
	case platform.VSphere != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.VSphere.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}

		certsSecret, err := r.getCredentialsSecret(pool, platform.VSphere.CertificatesSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}

		if _, ok := certsSecret.Data[".cacert"]; !ok {
			return nil, err
		}

		cloudBuilder := clusterresource.NewVSphereCloudBuilderFromSecret(credsSecret, certsSecret)
		cloudBuilder.Datacenter = platform.VSphere.Datacenter
		cloudBuilder.DefaultDatastore = platform.VSphere.DefaultDatastore
		cloudBuilder.VCenter = platform.VSphere.VCenter
		cloudBuilder.Cluster = platform.VSphere.Cluster
		cloudBuilder.Folder = platform.VSphere.Folder
		cloudBuilder.Network = platform.VSphere.Network

		return cloudBuilder, nil
	case platform.Ovirt != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.Ovirt.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}

		certsSecret, err := r.getCredentialsSecret(pool, platform.Ovirt.CertificatesSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}

		if _, ok := certsSecret.Data[".cacert"]; !ok {
			return nil, err
		}

		cloudBuilder := clusterresource.NewOvirtCloudBuilderFromSecret(credsSecret)
		cloudBuilder.StorageDomainID = platform.Ovirt.StorageDomainID
		cloudBuilder.ClusterID = platform.Ovirt.ClusterID
		cloudBuilder.NetworkName = platform.Ovirt.NetworkName

		return cloudBuilder, nil
	case platform.Nutanix != nil:
		credsSecret, err := r.getCredentialsSecret(pool, platform.Nutanix.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}

		cloudBuilder := clusterresource.NewNutanixCloudBuilder(credsSecret)
		cloudBuilder.PrismCentral = nutanix.PrismCentral{
			Endpoint: nutanix.PrismEndpoint{
				Address: cloudBuilder.PrismCentral.Endpoint.Address,
				Port:    cloudBuilder.PrismCentral.Endpoint.Port,
			},
			Username: cloudBuilder.PrismCentral.Username,
			Password: cloudBuilder.PrismCentral.Password,
		}

		for _, pe := range cloudBuilder.PrismElements {
			cloudBuilder.PrismElements = append(cloudBuilder.PrismElements, nutanix.PrismElement{
				UUID: pe.UUID,
				Endpoint: nutanix.PrismEndpoint{
					Address: pe.Endpoint.Address,
					Port:    pe.Endpoint.Port,
				},
				Name: pe.Name,
			})
		}

		cloudBuilder.SubnetUUIDs = platform.Nutanix.SubnetUUIDs

		return cloudBuilder, nil
	default:
		logger.Info("unsupported platform")
		return nil, errors.New("unsupported platform")
	}
}

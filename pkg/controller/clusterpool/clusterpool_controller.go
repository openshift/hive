package clusterpool

import (
	"context"
	"fmt"
	"math"
	"reflect"

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
)

const (
	ControllerName                  = hivev1.ClusterpoolControllerName
	finalizer                       = "hive.openshift.io/clusters"
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
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterPool")

	// clusterPoolConditions are the cluster pool conditions controlled or initialized by cluster pool controller
	clusterPoolConditions = []hivev1.ClusterPoolConditionType{
		hivev1.ClusterPoolMissingDependenciesCondition,
		hivev1.ClusterPoolCapacityAvailableCondition,
		hivev1.ClusterPoolAllClustersCurrentCondition,
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

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterPool, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("clusterpool-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Index ClusterDeployments by pool namespace/name
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &hivev1.ClusterDeployment{}, cdClusterPoolIndex,
		func(o client.Object) []string {
			cd := o.(*hivev1.ClusterDeployment)
			if poolRef := cd.Spec.ClusterPoolRef; poolRef != nil {
				return []string{poolKey(poolRef.Namespace, poolRef.PoolName)}
			}
			return []string{}
		}); err != nil {
		log.WithError(err).Error("Error indexing ClusterDeployments by ClusterPool")
		return err
	}

	// Index ClusterClaims by pool name
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &hivev1.ClusterClaim{}, claimClusterPoolIndex,
		func(o client.Object) []string {
			claim := o.(*hivev1.ClusterClaim)
			if poolName := claim.Spec.ClusterPoolName; poolName != "" {
				return []string{poolName}
			}
			return []string{}
		}); err != nil {
		log.WithError(err).Error("Error indexing ClusterClaims by ClusterPool")
		return err
	}

	// Watch for changes to ClusterPool
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterPool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployments originating from a pool:
	if err := r.watchClusterDeployments(c); err != nil {
		return err
	}

	// Watch for changes to ClusterClaims
	enqueuePoolForClaim := handler.EnqueueRequestsFromMapFunc(
		func(o client.Object) []reconcile.Request {
			claim, ok := o.(*hivev1.ClusterClaim)
			if !ok {
				return nil
			}
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{
					Namespace: claim.Namespace,
					Name:      claim.Spec.ClusterPoolName,
				},
			}}
		},
	)
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterClaim{}}, enqueuePoolForClaim); err != nil {
		return err
	}

	// Watch for changes to the hive cluster pool admin RoleBindings
	if err := c.Watch(
		&source.Kind{Type: &rbacv1.RoleBinding{}},
		handler.EnqueueRequestsFromMapFunc(
			requestsForRBACResources(r.Client, r.logger)),
	); err != nil {
		return err
	}

	// Watch for changes to the hive-cluster-pool-admin-binding RoleBinding
	if err := c.Watch(
		&source.Kind{Type: &rbacv1.RoleBinding{}},
		handler.EnqueueRequestsFromMapFunc(
			requestsForCDRBACResources(r.Client, clusterPoolAdminRoleBindingName, r.logger)),
	); err != nil {
		return err
	}

	return nil
}

func requestsForCDRBACResources(c client.Client, resourceName string, logger log.FieldLogger) handler.MapFunc {
	return func(o client.Object) []reconcile.Request {
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

func requestsForRBACResources(c client.Client, logger log.FieldLogger) handler.MapFunc {
	return func(o client.Object) []reconcile.Request {
		binding, ok := o.(*rbacv1.RoleBinding)
		if !ok {
			return nil
		}
		if binding.RoleRef.Kind != "ClusterRole" || binding.RoleRef.Name != clusterPoolAdminRoleName {
			return nil
		}

		cpList := &hivev1.ClusterPoolList{}
		if err := c.List(context.Background(), cpList, client.InNamespace(o.GetNamespace())); err != nil {
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

	// Initialize cluster pool conditions if not set
	newConditions := controllerutils.InitializeClusterPoolConditions(clp.Status.Conditions, clusterPoolConditions)
	if len(newConditions) > len(clp.Status.Conditions) {
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

	claims.SyncClusterDeploymentAssignments(r.Client, cds, logger)
	cds.SyncClaimAssignments(r.Client, claims, logger)

	origStatus := clp.Status.DeepCopy()
	clp.Status.Size = int32(len(cds.Installing()) + len(cds.Assignable()) + len(cds.Broken()))
	clp.Status.Ready = int32(len(cds.Assignable()))
	if !reflect.DeepEqual(origStatus, &clp.Status) {
		if err := r.Status().Update(context.Background(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterPool status")
			return reconcile.Result{}, errors.Wrap(err, "could not update ClusterPool status")
		}
	}

	availableCapacity := math.MaxInt32
	if clp.Spec.MaxSize != nil {
		availableCapacity = int(*clp.Spec.MaxSize) - cds.Total()
		numUnassigned := len(cds.Installing()) + len(cds.Assignable()) + len(cds.Broken())
		numAssigned := cds.NumAssigned()
		if availableCapacity <= 0 {
			logger.WithFields(log.Fields{
				"UnclaimedSize": numUnassigned,
				"ClaimedSize":   numAssigned,
				"Capacity":      *clp.Spec.MaxSize,
			}).Info("Cannot add more clusters because no capacity available.")
		}
	}
	if err := r.setAvailableCapacityCondition(clp, availableCapacity > 0, logger); err != nil {
		logger.WithError(err).Error("error setting CapacityAvailable condition")
		return reconcile.Result{}, err
	}

	// reserveSize is the number of clusters that the pool currently has in reserve
	reserveSize := len(cds.Installing()) + len(cds.Assignable()) + len(cds.Broken()) - len(claims.Unassigned())

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

	switch drift := reserveSize - int(clp.Spec.Size); {
	// activity quota exceeded, so no action
	case availableCurrent <= 0:
		logger.WithFields(log.Fields{
			"MaxConcurrent": *clp.Spec.MaxConcurrent,
			"Available":     availableCurrent,
		}).Info("Cannot create/delete clusters as max concurrent quota exceeded.")
	// If too few, create new InstallConfig and ClusterDeployment.
	case drift < 0:
		if availableCapacity <= 0 {
			break
		}
		toAdd := minIntVarible(-drift, availableCapacity, availableCurrent)
		// TODO: Do this in cdLookup so we can add the new CDs to Installing(). For now it's okay
		// because the new CDs are guaranteed to have a matching PoolVersion, and that's the only
		// thing we need to keep consistent.
		if err := r.addClusters(clp, poolVersion, toAdd, logger); err != nil {
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

	// One more (possible) status update: wait until the end to detect whether all unassigned CDs
	// are current with the pool config, since assigning or deleting CDs may eliminate the last
	// one that isn't.
	if err := setCDsCurrentCondition(r.Client, cds, clp, poolVersion); err != nil {
		log.WithError(err).Error("error updating 'ClusterDeployments current' status")
		return reconcile.Result{}, err
	}

	if err := r.reconcileRBAC(clp, logger); err != nil {
		log.WithError(err).Error("error reconciling RBAC")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
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
	newClusterCount int,
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
		if err := r.createCluster(clp, cloudBuilder, pullSecret, installConfigTemplate, poolVersion, logger); err != nil {
			return err
		}
	}

	return nil
}

func (r *ReconcileClusterPool) createCluster(
	clp *hivev1.ClusterPool,
	cloudBuilder clusterresource.CloudBuilder,
	pullSecret string,
	installConfigTemplate string,
	poolVersion string,
	logger log.FieldLogger,
) error {
	ns, err := r.createRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return err
	}
	logger.WithField("cluster", ns.Name).Info("Creating new cluster")

	annotations := clp.Spec.Annotations
	// Annotate the CD so we can distinguish "stale" CDs
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[constants.ClusterDeploymentPoolSpecHashAnnotation] = poolVersion

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
		Labels:                clp.Spec.Labels,
		Annotations:           annotations,
		InstallConfigTemplate: installConfigTemplate,
		SkipMachinePools:      clp.Spec.SkipMachinePools,
	}

	if clp.Spec.HibernateAfter != nil {
		builder.HibernateAfter = &clp.Spec.HibernateAfter.Duration
	}

	objs, err := builder.Build()
	if err != nil {
		return errors.Wrap(err, "error building resources")
	}

	poolKey := types.NamespacedName{Namespace: clp.Namespace, Name: clp.Name}.String()
	r.expectations.ExpectCreations(poolKey, 1)
	// Add the ClusterPoolRef to the ClusterDeployment, and move it to the end of the slice.
	for i, obj := range objs {
		cd, ok := obj.(*hivev1.ClusterDeployment)
		if !ok {
			continue
		}
		poolRef := poolReference(clp)
		cd.Spec.ClusterPoolRef = &poolRef
		cd.Spec.PowerState = hivev1.HibernatingClusterPowerState
		lastIndex := len(objs) - 1
		objs[i], objs[lastIndex] = objs[lastIndex], objs[i]
	}
	// Create the resources.
	for _, obj := range objs {
		if err := r.Client.Create(context.Background(), obj.(client.Object)); err != nil {
			r.expectations.CreationObserved(poolKey)
			return err
		}
	}

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

func (r *ReconcileClusterPool) deleteExcessClusters(
	cds *cdCollection,
	deletionsNeeded int,
	logger log.FieldLogger,
) error {

	logger.WithField("deletionsNeeded", deletionsNeeded).Info("deleting excess clusters")
	clustersToDelete := make([]*hivev1.ClusterDeployment, 0, deletionsNeeded)
	installingClusters := cds.Installing()
	readyClusters := cds.Assignable()
	if deletionsNeeded < len(installingClusters) {
		clustersToDelete = installingClusters[:deletionsNeeded]
	} else {
		clustersToDelete = append(clustersToDelete, installingClusters...)
		deletionsOfInstalledClustersNeeded := deletionsNeeded - len(installingClusters)
		if deletionsOfInstalledClustersNeeded <= len(readyClusters) {
			clustersToDelete = append(clustersToDelete, readyClusters[:deletionsOfInstalledClustersNeeded]...)
		} else {
			logger.WithField("deletionsNeeded", deletionsNeeded).
				WithField("installingClusters", len(installingClusters)).
				WithField("installedClusters", len(readyClusters)).
				Error("trying to delete more clusters than there are available")
			clustersToDelete = append(clustersToDelete, readyClusters...)
		}
	}
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
	// Don't care about the poolVersion here since we're deleting everything.
	cds, err := getAllClusterDeploymentsForPool(r.Client, pool, "", logger)
	if err != nil {
		return err
	}
	for _, cd := range append(cds.Assignable(), cds.Installing()...) {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if err := cds.Delete(r.Client, cd.Name); err != nil {
			logger.WithError(err).WithField("cluster", cd.Name).Log(controllerutils.LogLevel(err), "could not delete ClusterDeployment")
			return errors.Wrap(err, "could not delete ClusterDeployment")
		}
	}
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
	// TODO: OpenStack, VMware, and Ovirt.
	default:
		logger.Info("unsupported platform")
		return nil, errors.New("unsupported platform")
	}
}

package clusterpool

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

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
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterPool")
)

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
	enqueuePoolForClaim := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(
			func(o handler.MapObject) []reconcile.Request {
				claim, ok := o.Object.(*hivev1.ClusterClaim)
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
		),
	}
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterClaim{}}, enqueuePoolForClaim); err != nil {
		return err
	}

	// Watch for changes to the hive cluster pool admin RoleBindings
	if err := c.Watch(
		&source.Kind{Type: &rbacv1.RoleBinding{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: requestsForRBACResources(r.Client, r.logger),
		},
	); err != nil {
		return err
	}

	// Watch for changes to the hive-cluster-pool-admin-binding RoleBinding
	if err := c.Watch(
		&source.Kind{Type: &rbacv1.RoleBinding{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: requestsForCDRBACResources(r.Client, clusterPoolAdminRoleBindingName, r.logger),
		},
	); err != nil {
		return err
	}

	return nil
}

func requestsForCDRBACResources(c client.Client, resourceName string, logger log.FieldLogger) handler.ToRequestsFunc {
	return func(o handler.MapObject) []reconcile.Request {
		if o.Meta.GetName() != resourceName {
			return nil
		}
		clusterName := o.Meta.GetNamespace()
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

func requestsForRBACResources(c client.Client, logger log.FieldLogger) handler.ToRequestsFunc {
	return func(o handler.MapObject) []reconcile.Request {
		binding, ok := o.Object.(*rbacv1.RoleBinding)
		if !ok {
			return nil
		}
		if binding.RoleRef.Kind != "ClusterRole" || binding.RoleRef.Name != clusterPoolAdminRoleName {
			return nil
		}

		cpList := &hivev1.ClusterPoolList{}
		if err := c.List(context.Background(), cpList, client.InNamespace(o.Meta.GetNamespace())); err != nil {
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
func (r *ReconcileClusterPool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterPool", request.NamespacedName)
	logger.Infof("reconciling cluster pool")
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

	// Find all ClusterDeployments from this pool:
	claimedCDs, unClaminedCDs, err := r.getAllClusterDeploymentsForPool(clp, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	var toRemoveClaimedCDs []*hivev1.ClusterDeployment
	numberOfDeletingClaimedCDs := 0
	for _, cd := range claimedCDs {
		toRemove := controllerutils.IsClaimedClusterMarkedForRemoval(cd)
		switch {
		case cd.DeletionTimestamp != nil:
			numberOfDeletingClaimedCDs++
		case toRemove:
			toRemoveClaimedCDs = append(toRemoveClaimedCDs, cd)
		}
	}

	var installingCDs []*hivev1.ClusterDeployment
	var readyCDs []*hivev1.ClusterDeployment
	numberOfDeletingCDs := 0
	for _, cd := range unClaminedCDs {
		switch {
		case cd.DeletionTimestamp != nil:
			numberOfDeletingCDs++
		case !cd.Spec.Installed:
			installingCDs = append(installingCDs, cd)
		default:
			readyCDs = append(readyCDs, cd)
		}
	}

	logger.WithFields(log.Fields{
		"installing": len(installingCDs),
		"deleting":   numberOfDeletingCDs,
		"total":      len(unClaminedCDs),
		"ready":      len(readyCDs),
	}).Debug("found clusters for ClusterPool")

	origStatus := clp.Status.DeepCopy()
	clp.Status.Size = int32(len(installingCDs) + len(readyCDs))
	clp.Status.Ready = int32(len(readyCDs))
	if !reflect.DeepEqual(origStatus, &clp.Status) {
		if err := r.Status().Update(context.Background(), clp); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterPool status")
			return reconcile.Result{}, errors.Wrap(err, "could not update ClusterPool status")
		}
	}

	availableCapacity := math.MaxInt32
	if clp.Spec.MaxSize != nil {
		availableCapacity = int(*clp.Spec.MaxSize) - len(unClaminedCDs) - len(claimedCDs)
		if availableCapacity <= 0 {
			logger.WithFields(log.Fields{
				"lUnclaimedSize": len(unClaminedCDs),
				"ClaimedSize":    len(claimedCDs),
				"Capacity":       *clp.Spec.MaxSize,
			}).Info("Cannot add more clusters because no capacity available.")
		}
	}
	if err := r.setAvailableCapacityCondition(clp, availableCapacity > 0, logger); err != nil {
		logger.WithError(err).Error("error setting CapacityAvailable condition")
		return reconcile.Result{}, err
	}

	pendingClaims, err := r.getAllPendingClusterClaims(clp, logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	logger.WithField("count", len(pendingClaims)).Debug("found pending claims for ClusterPool")

	// reserveSize is the number of clusters that the pool currently has in reserve
	reserveSize := len(installingCDs) + len(readyCDs) - len(pendingClaims)

	readyCDs, err = r.assignClustersToClaims(pendingClaims, readyCDs, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	availableCurrent := math.MaxInt32
	if clp.Spec.MaxConcurrent != nil {
		availableCurrent = int(*clp.Spec.MaxConcurrent) - len(installingCDs) - numberOfDeletingCDs - numberOfDeletingClaimedCDs
		if availableCurrent < 0 {
			availableCurrent = 0
		}
	}

	// remove clusters that were previously claimed but now not required.
	toDel := minIntVarible(len(toRemoveClaimedCDs), availableCurrent)
	for _, cd := range toRemoveClaimedCDs[:toDel] {
		cdLog := logger.WithField("cluster", cd.Name)
		cdLog.Info("deleting cluster deployment for previous claim")
		if err := r.Client.Delete(context.Background(), cd); err != nil {
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
	// If too many, delete some.
	case drift > 0:
		toDel := minIntVarible(drift, availableCurrent)
		if err := r.deleteExcessClusters(installingCDs, readyCDs, toDel, logger); err != nil {
			return reconcile.Result{}, err
		}
	// If too few, create new InstallConfig and ClusterDeployment.
	case drift < 0:
		if availableCapacity <= 0 {
			break
		}
		toAdd := minIntVarible(-drift, availableCapacity, availableCurrent)
		if err := r.addClusters(clp, toAdd, logger); err != nil {
			log.WithError(err).Error("error adding clusters")
			return reconcile.Result{}, err
		}
	}

	if err := r.reconcileRBAC(clp, logger); err != nil {
		log.WithError(err).Error("error reconciling RBAC")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
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
		if err := r.createCluster(clp, cloudBuilder, pullSecret, installConfigTemplate, logger); err != nil {
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
	logger log.FieldLogger,
) error {
	ns, err := r.createRandomNamespace(clp)
	if err != nil {
		logger.WithError(err).Error("error obtaining random namespace")
		return err
	}
	logger.WithField("cluster", ns.Name).Info("Creating new cluster")

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
		if err := r.Client.Create(context.Background(), obj); err != nil {
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
	installingClusters []*hivev1.ClusterDeployment,
	readyClusters []*hivev1.ClusterDeployment,
	deletionsNeeded int,
	logger log.FieldLogger,
) error {

	logger.WithField("deletionsNeeded", deletionsNeeded).Info("deleting excess clusters")
	clustersToDelete := make([]*hivev1.ClusterDeployment, 0, deletionsNeeded)
	if deletionsNeeded < len(installingClusters) {
		// Sort the installing clusters in order by creation timestamp from newest to oldest. This has the effect of
		// prioritizing deleting those clusters that have the longest time until they are installed.
		sort.Slice(installingClusters, func(i, j int) bool {
			return installingClusters[i].CreationTimestamp.After(installingClusters[j].CreationTimestamp.Time)
		})
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
		if err := r.Client.Delete(context.Background(), cd); err != nil {
			logger.WithError(err).Error("error deleting cluster deployment")
			return err
		}
	}
	logger.Info("no more deletions required")
	return nil
}

func (r *ReconcileClusterPool) reconcileDeletedPool(pool *hivev1.ClusterPool, logger log.FieldLogger) error {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return nil
	}
	_, unClaimedCDs, err := r.getAllClusterDeploymentsForPool(pool, logger)
	if err != nil {
		return err
	}
	for _, cd := range unClaimedCDs {
		if cd.DeletionTimestamp != nil {
			continue
		}
		if err := r.Delete(context.Background(), cd); err != nil {
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

func (r *ReconcileClusterPool) getAllClusterDeploymentsForPool(pool *hivev1.ClusterPool, logger log.FieldLogger) (claimed, unclaimed []*hivev1.ClusterDeployment, err error) {
	cdList := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), cdList); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments")
		return nil, nil, err
	}
	for i, cd := range cdList.Items {
		if refInCD := cd.Spec.ClusterPoolRef; refInCD != nil {
			if refInCD.Namespace == pool.Namespace && refInCD.PoolName == pool.Name {
				if len(refInCD.ClaimName) > 0 {
					claimed = append(claimed, &cdList.Items[i])
				} else {
					unclaimed = append(unclaimed, &cdList.Items[i])
				}
			}
		}
	}
	return claimed, unclaimed, nil
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
		credsSecret, err := r.getCredentialsSecret(pool, platform.AWS.CredentialsSecretRef.Name, logger)
		if err != nil {
			return nil, err
		}
		cloudBuilder := clusterresource.NewAWSCloudBuilderFromSecret(credsSecret)
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
		return cloudBuilder, nil
	// TODO: OpenStack, VMware, and Ovirt.
	default:
		logger.Info("unsupported platform")
		return nil, errors.New("unsupported platform")
	}
}

// getAllPendingClusterClaims returns all of the ClusterClaims that are requesting clusters from the specified pool.
// The claims are returned in order of creation time, from oldest to youngest.
func (r *ReconcileClusterPool) getAllPendingClusterClaims(pool *hivev1.ClusterPool, logger log.FieldLogger) ([]*hivev1.ClusterClaim, error) {
	claimsList := &hivev1.ClusterClaimList{}
	if err := r.Client.List(context.Background(), claimsList, client.InNamespace(pool.Namespace)); err != nil {
		logger.WithError(err).Error("error listing ClusterClaims")
		return nil, err
	}
	var pendingClaims []*hivev1.ClusterClaim
	for i, claim := range claimsList.Items {
		// skip claims for other pools
		if claim.Spec.ClusterPoolName != pool.Name {
			continue
		}
		// skip claims that have been assigned already
		if claim.Spec.Namespace != "" {
			continue
		}
		pendingClaims = append(pendingClaims, &claimsList.Items[i])
	}
	sort.Slice(
		pendingClaims,
		func(i, j int) bool {
			return pendingClaims[i].CreationTimestamp.Before(&pendingClaims[j].CreationTimestamp)
		},
	)
	return pendingClaims, nil
}

func (r *ReconcileClusterPool) assignClustersToClaims(claims []*hivev1.ClusterClaim, cds []*hivev1.ClusterDeployment, logger log.FieldLogger) ([]*hivev1.ClusterDeployment, error) {
	for _, claim := range claims {
		logger := logger.WithField("claim", claim.Name)
		var conds []hivev1.ClusterClaimCondition
		var statusChanged bool
		if len(cds) > 0 {
			claim.Spec.Namespace = cds[0].Namespace
			cds = cds[1:]
			logger.WithField("cluster", claim.Spec.Namespace).Info("assigning cluster to claim")
			if err := r.Update(context.Background(), claim); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not assign cluster to claim")
				return cds, err
			}
			conds = controllerutils.SetClusterClaimCondition(
				claim.Status.Conditions,
				hivev1.ClusterClaimPendingCondition,
				corev1.ConditionTrue,
				"ClusterAssigned",
				"Cluster assigned to ClusterClaim, awaiting claim",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
			statusChanged = true
		} else {
			logger.Debug("no clusters ready to assign to claim")
			conds, statusChanged = controllerutils.SetClusterClaimConditionWithChangeCheck(
				claim.Status.Conditions,
				hivev1.ClusterClaimPendingCondition,
				corev1.ConditionTrue,
				"NoClusters",
				"No clusters in pool are ready to be claimed",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)
		}
		if statusChanged {
			claim.Status.Conditions = conds
			if err := r.Status().Update(context.Background(), claim); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status of ClusterClaim")
				return cds, err
			}
		}
	}
	return cds, nil
}

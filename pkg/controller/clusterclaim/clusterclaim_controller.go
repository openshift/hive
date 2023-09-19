package clusterclaim

import (
	"context"
	"reflect"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName                = hivev1.ClusterClaimControllerName
	finalizer                     = "hive.openshift.io/claim"
	hiveClaimOwnerRoleName        = "hive-claim-owner"
	hiveClaimOwnerRoleBindingName = "hive-claim-owner"
)

// clusterClaimConditions are the cluster claim conditions controlled or initialized by cluster claim controller
var clusterClaimConditions = []hivev1.ClusterClaimConditionType{
	hivev1.ClusterClaimPendingCondition,
	hivev1.ClusterRunningCondition,
}

// Add creates a new ClusterClaim Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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

// NewReconciler returns a new ReconcileClusterClaim
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *ReconcileClusterClaim {
	logger := log.WithField("controller", ControllerName)
	return &ReconcileClusterClaim{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		logger: logger,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterClaim, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("clusterclaim-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, r.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterClaim
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterClaim{}), &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(
			handler.EnqueueRequestsFromMapFunc(requestsForClusterDeployment),
			controllerutils.IsClusterDeploymentErrorUpdateEvent)); err != nil {
		return err
	}

	// Watch for changes to the hive-claim-owner Role
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &rbacv1.Role{}),
		handler.EnqueueRequestsFromMapFunc(requestsForRBACResources(r.Client, hiveClaimOwnerRoleName, r.logger))); err != nil {
		return err
	}

	// Watch for changes to the hive-claim-owner RoleBinding
	if err := c.Watch(
		source.Kind(mgr.GetCache(), &rbacv1.RoleBinding{}),
		handler.EnqueueRequestsFromMapFunc(requestsForRBACResources(r.Client, hiveClaimOwnerRoleBindingName, r.logger))); err != nil {
		return err
	}

	return nil
}

func claimForClusterDeployment(cd *hivev1.ClusterDeployment) *types.NamespacedName {
	if cd.Spec.ClusterPoolRef == nil {
		return nil
	}
	if cd.Spec.ClusterPoolRef.ClaimName == "" {
		return nil
	}
	return &types.NamespacedName{
		Namespace: cd.Spec.ClusterPoolRef.Namespace,
		Name:      cd.Spec.ClusterPoolRef.ClaimName,
	}
}

func requestsForClusterDeployment(ctx context.Context, o client.Object) []reconcile.Request {
	cd, ok := o.(*hivev1.ClusterDeployment)
	if !ok {
		return nil
	}
	claim := claimForClusterDeployment(cd)
	if claim == nil {
		return nil
	}
	return []reconcile.Request{{NamespacedName: *claim}}
}

func requestsForRBACResources(c client.Client, resourceName string, logger log.FieldLogger) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		if o.GetName() != resourceName {
			return nil
		}
		clusterName := o.GetNamespace()
		cd := &hivev1.ClusterDeployment{}
		if err := c.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get ClusterDeployment for RBAC resource")
			return nil
		}
		claim := claimForClusterDeployment(cd)
		if claim == nil {
			return nil
		}
		return []reconcile.Request{{NamespacedName: *claim}}
	}
}

var _ reconcile.Reconciler = &ReconcileClusterClaim{}

// ReconcileClusterClaim reconciles a CLusterClaim object
type ReconcileClusterClaim struct {
	client.Client
	logger log.FieldLogger
}

// Reconcile reconciles a ClusterClaim.
func (r *ReconcileClusterClaim) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterClaim", request.NamespacedName)
	logger.Infof("reconciling cluster claim")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterClaim instance
	claim := &hivev1.ClusterClaim{}
	err := r.Get(context.TODO(), request.NamespacedName, claim)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("claim not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.WithError(err).Error("error getting ClusterClaim")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: claim}, logger)

	// Initialize cluster claim conditions if not set
	newConditions, changed := controllerutils.InitializeClusterClaimConditions(claim.Status.Conditions, clusterClaimConditions)
	if changed {
		claim.Status.Conditions = newConditions
		logger.Infof("initializing cluster claim conditions")
		if err := r.Status().Update(context.TODO(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster claim status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if claim.DeletionTimestamp != nil {
		return r.reconcileDeletedClaim(claim, logger)
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(claim, finalizer) {
		logger.Debug("adding finalizer to ClusterClaim")
		controllerutils.AddFinalizer(claim, finalizer)
		if err := r.Update(context.Background(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer to ClusterClaim")
			return reconcile.Result{}, err
		}
	}

	clusterName := claim.Spec.Namespace
	if clusterName == "" {
		logger.Debug("claim has not yet been assigned a cluster")
		return reconcile.Result{}, nil
	}

	logger = logger.WithField("cluster", clusterName)

	poolLifetime, err := r.clusterPoolLifetimeForClaim(claim, logger)
	if err != nil {
		logger.Log(controllerutils.LogLevel(err), "error getting cluster pool lifetime")
		return reconcile.Result{}, err
	}
	lifetime := getClaimLifetime(poolLifetime, claim.Spec.Lifetime)

	if (lifetime != nil) != (claim.Status.Lifetime != nil) ||
		lifetime != nil && claim.Status.Lifetime != nil && lifetime.Duration != claim.Status.Lifetime.Duration {
		claim.Status.Lifetime = lifetime
		if err := r.Status().Update(context.Background(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update ClusterClaim lifetime")
			return reconcile.Result{}, errors.Wrap(err, "could not update ClusterClaim lifetime")
		}
	}

	// Delete ClusterClaim after its lifetime elapses
	if lifetime != nil {
		logger.WithField("lifetime", lifetime).Debug("checking whether lifetime of ClusterClaim has elapsed")
		pendingCond := controllerutils.FindCondition(claim.Status.Conditions, hivev1.ClusterClaimPendingCondition)
		if pendingCond.Status == corev1.ConditionFalse {
			if timeSinceAssigned := time.Since(pendingCond.LastTransitionTime.Time); timeSinceAssigned >= lifetime.Duration {
				logger.WithField("timeSinceAssigned", timeSinceAssigned).
					WithField("lifetime", lifetime).
					Info("deleting ClusterClaim because its lifetime has elapsed")
				if err := r.Delete(context.Background(), claim); err != nil {
					logger.WithError(err).Log(controllerutils.LogLevel(err), "could not delete ClusterClaim")
					return reconcile.Result{}, errors.Wrap(err, "could not delete ClusterClaim")
				}
				return reconcile.Result{}, nil
			}
			defer func() {
				result, returnErr = controllerutils.EnsureRequeueAtLeastWithin(
					lifetime.Duration-time.Since(pendingCond.LastTransitionTime.Time),
					result,
					returnErr,
				)
			}()
		}
	}

	cd := &hivev1.ClusterDeployment{}
	switch err := r.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); {
	case apierrors.IsNotFound(err):
		return r.reconcileForDeletedCluster(claim, logger)
	case cd.DeletionTimestamp != nil || controllerutils.IsClusterMarkedForRemoval(cd):
		return r.setDeletingStatus(claim, logger)
	case err != nil:
		logger.Log(controllerutils.LogLevel(err), "error getting ClusterDeployment")
		return reconcile.Result{}, err
	}

	switch cd.Spec.ClusterPoolRef.ClaimName {
	case "":
		logger.Debugf("clusterdeployment %s has not yet been assigned to claim", cd.Name)
		return reconcile.Result{}, nil
	case claim.Name:
		return r.reconcileForExistingAssignment(claim, cd, logger)
	default:
		return r.reconcileForAssignmentConflict(claim, logger)
	}
}

func (r *ReconcileClusterClaim) setDeletingStatus(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	if conds, changed := controllerutils.SetClusterClaimConditionWithChangeCheck(
		claim.Status.Conditions,
		hivev1.ClusterRunningCondition,
		corev1.ConditionFalse,
		"ClusterDeleting",
		"Assigned cluster has been marked for deletion",
		controllerutils.UpdateConditionIfReasonOrMessageChange); changed {
		claim.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status of ClusterClaim")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// getClaimLifetime returns the lifetime for a claim taking into account the lifetime set on the pool
// and the claim.
// if no lifetime is set on the claim, the default lifetime for the pool is used if set.
// if maximum lifetime for the pool, the lifetime is minimum of this maximum and lifetime from above.
func getClaimLifetime(poolLifetime *hivev1.ClusterPoolClaimLifetime, claimLifetime *metav1.Duration) *metav1.Duration {
	var lifetime *metav1.Duration
	if poolLifetime != nil && poolLifetime.Default != nil {
		lifetime = poolLifetime.Default
	}
	if claimLifetime != nil {
		lifetime = claimLifetime
	}
	if poolLifetime != nil && poolLifetime.Maximum != nil {
		if lifetime == nil ||
			(poolLifetime.Maximum.Duration < lifetime.Duration) {
			lifetime = poolLifetime.Maximum
		}

	}
	return lifetime
}

// clusterPoolLifetimeForClaim returns the default and max lifetimes for the cluster pool the claim belongs to.
func (r *ReconcileClusterClaim) clusterPoolLifetimeForClaim(claim *hivev1.ClusterClaim, logger log.FieldLogger) (*hivev1.ClusterPoolClaimLifetime, error) {
	// Fetch the ClusterPool instance
	clp := &hivev1.ClusterPool{}
	// claims exists in the same namespace as the pool
	key := client.ObjectKey{Namespace: claim.Namespace, Name: claim.Spec.ClusterPoolName}
	err := r.Get(context.TODO(), key, clp)
	if apierrors.IsNotFound(err) {
		logger.WithField("pool", key).WithField("claim", claim.Name).Info("cluster pool no longer exists")
		// since there is no pool no lifetime can be extracted. this is a valid state.
		return nil, nil
	}
	if err != nil {
		log.WithError(err).Error("error reading cluster pool")
		return nil, errors.Wrap(err, "failed to get the pool")
	}
	return clp.Spec.ClaimLifetime, nil
}

func (r *ReconcileClusterClaim) reconcileDeletedClaim(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(claim, finalizer) {
		return reconcile.Result{}, nil
	}

	claimReadyForDeletion, err := r.cleanupResources(claim, logger)
	if err != nil {
		return reconcile.Result{}, err
	}
	if !claimReadyForDeletion {
		logger.Info("waiting for associated resources to be deleted")
		// Don't requeue; our watches for CD, role, and rolebinding will trigger the Reconcile as
		// those resources disappear.
		return reconcile.Result{}, nil
	}

	logger.Info("removing finalizer from ClusterClaim")
	controllerutils.DeleteFinalizer(claim, finalizer)
	if err := r.Update(context.Background(), claim); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer from ClusterClaim")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// cleanupResources deletes the ClusterDeployment, Role, and RoleBinding associated with this claim.
// (The CD deletion is via an annotation that's acted on by the clusterpoolnamespace controller.)
// The first return value is true iff the associated resources are actually gone (as opposed to
// just marked for deletion).
func (r *ReconcileClusterClaim) cleanupResources(claim *hivev1.ClusterClaim, logger log.FieldLogger) (bool, error) {
	clusterName := claim.Spec.Namespace
	if clusterName == "" {
		logger.Info("no resources to clean up since claim was never assigned a cluster")
		return true, nil
	}
	logger = logger.WithField("cluster", clusterName)

	var cdGone, rolebindingGone, roleGone bool
	var err error

	cd := &hivev1.ClusterDeployment{}
	switch err := r.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); {
	case apierrors.IsNotFound(err):
		logger.Info("cluster does not exist")
		cdGone = true
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "error getting ClusterDeployment")
		return false, err
	}

	if poolRef := cd.Spec.ClusterPoolRef; poolRef == nil || poolRef.Namespace != claim.Namespace || poolRef.ClaimName != claim.Name {
		logger.Info("assigned cluster was not claimed")
		return true, nil
	}

	// Delete RoleBinding
	rolebindingGone, err = resource.DeleteAnyExistingObject(
		r,
		client.ObjectKey{Namespace: clusterName, Name: hiveClaimOwnerRoleBindingName},
		&rbacv1.RoleBinding{},
		logger,
	)
	if err != nil {
		return false, err
	}

	// Delete Role
	roleGone, err = resource.DeleteAnyExistingObject(
		r,
		client.ObjectKey{Namespace: clusterName, Name: hiveClaimOwnerRoleName},
		&rbacv1.Role{},
		logger,
	)
	if err != nil {
		return false, err
	}

	// Delete ClusterDeployment
	if !cdGone && cd.DeletionTimestamp == nil && !controllerutils.IsClusterMarkedForRemoval(cd) {
		logger.Info("marking clusterDeployment for deletion by the clusterpool controller")
		controllerutils.MarkClusterForRemoval(cd)
		if err := r.Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error updating ClusterDeployment to mark it for deletion")
			return false, err
		}
	}

	return cdGone && rolebindingGone && roleGone, nil
}

func (r *ReconcileClusterClaim) reconcileForDeletedCluster(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("assigned cluster has been deleted")
	conds, changed := controllerutils.SetClusterClaimConditionWithChangeCheck(
		claim.Status.Conditions,
		hivev1.ClusterRunningCondition,
		corev1.ConditionFalse,
		"ClusterDeleted",
		"Assigned cluster has been deleted",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
		claim.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterClaim) reconcileForExistingAssignment(claim *hivev1.ClusterClaim, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("claim has existing cluster assignment")
	if err := r.createRBAC(claim, cd, logger); err != nil {
		return reconcile.Result{}, err
	}
	var statusChanged bool
	var changed bool
	conds := claim.Status.Conditions

	conds, changed = controllerutils.SetClusterClaimConditionWithChangeCheck(
		conds,
		hivev1.ClusterClaimPendingCondition,
		corev1.ConditionFalse,
		"ClusterClaimed",
		"Cluster claimed",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	statusChanged = statusChanged || changed

	if cd.Status.PowerState == hivev1.ClusterPowerStateRunning {
		conds, changed = controllerutils.SetClusterClaimConditionWithChangeCheck(
			conds,
			hivev1.ClusterRunningCondition,
			corev1.ConditionTrue,
			"Running",
			"Cluster is running",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		statusChanged = statusChanged || changed
	} else {
		log.Debug("waiting for cluster to be running")
		conds, changed = controllerutils.SetClusterClaimConditionWithChangeCheck(
			conds,
			hivev1.ClusterRunningCondition,
			corev1.ConditionFalse,
			"Resuming",
			"Waiting for cluster to be running",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		statusChanged = statusChanged || changed
	}
	if statusChanged {
		log.Debug("conditions changed, updating claim status")
		claim.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), claim); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status of ClusterClaim")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterClaim) reconcileForAssignmentConflict(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Info("claim assigned a cluster that has already been claimed by another ClusterClaim")
	claim.Spec.Namespace = ""
	// TODO: Update status handling https://issues.redhat.com/browse/HIVE-2274
	claim.Status.Conditions = controllerutils.SetClusterClaimCondition(
		claim.Status.Conditions,
		hivev1.ClusterClaimPendingCondition,
		corev1.ConditionTrue,
		"AssignmentConflict",
		"Assigned cluster was claimed by a different ClusterClaim",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if err := r.Update(context.Background(), claim); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update spec of ClusterClaim")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterClaim) createRBAC(claim *hivev1.ClusterClaim, cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	if len(claim.Spec.Subjects) == 0 {
		logger.Debug("not creating RBAC since claim does not specify any subjects")
		return nil
	}
	if cd.Spec.ClusterMetadata == nil {
		return errors.New("ClusterDeployment does not have ClusterMetadata")
	}
	if err := r.applyHiveClaimOwnerRole(claim, cd, logger); err != nil {
		return err
	}
	if err := r.applyHiveClaimOwnerRoleBinding(claim, cd, logger); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterClaim) applyHiveClaimOwnerRole(claim *hivev1.ClusterClaim, cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	desiredRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cd.Namespace,
			Name:      hiveClaimOwnerRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			// Allow full access to all Hive resources
			{
				APIGroups: []string{hivev1.HiveAPIGroup},
				Resources: []string{rbacv1.ResourceAll},
				Verbs:     []string{rbacv1.VerbAll},
			},
			// Allow read access to the kubeconfig and admin password secrets
			{
				APIGroups: []string{corev1.GroupName},
				Resources: []string{"secrets"},
				ResourceNames: []string{
					cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name,
					cd.Spec.ClusterMetadata.AdminPasswordSecretRef.Name,
				},
				Verbs: []string{"get"},
			},
		},
	}
	observedRole := &rbacv1.Role{}
	updateRole := func() bool {
		if reflect.DeepEqual(desiredRole.Rules, observedRole.Rules) {
			return false
		}
		observedRole.Rules = desiredRole.Rules
		return true
	}
	if err := r.applyResource(desiredRole, observedRole, updateRole, logger); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterClaim) applyHiveClaimOwnerRoleBinding(claim *hivev1.ClusterClaim, cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	desiredRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cd.Namespace,
			Name:      hiveClaimOwnerRoleBindingName,
		},
		Subjects: claim.Spec.Subjects,
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     hiveClaimOwnerRoleName,
		},
	}
	observedRoleBinding := &rbacv1.RoleBinding{}
	updateRole := func() bool {
		if reflect.DeepEqual(desiredRoleBinding.Subjects, observedRoleBinding.Subjects) &&
			reflect.DeepEqual(desiredRoleBinding.RoleRef, observedRoleBinding.RoleRef) {
			return false
		}
		observedRoleBinding.Subjects = desiredRoleBinding.Subjects
		observedRoleBinding.RoleRef = desiredRoleBinding.RoleRef
		return true
	}
	if err := r.applyResource(desiredRoleBinding, observedRoleBinding, updateRole, logger); err != nil {
		return err
	}
	return nil
}

func (r *ReconcileClusterClaim) applyResource(desired, observed hivev1.MetaRuntimeObject, update func() bool, logger log.FieldLogger) error {
	key := client.ObjectKey{
		Namespace: desired.GetNamespace(),
		Name:      desired.GetName(),
	}
	logger = logger.WithField("resource", key)
	switch err := r.Get(context.Background(), key, observed); {
	case apierrors.IsNotFound(err):
		logger.Info("creating resource")
		if err := r.Create(context.Background(), desired); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create resource")
			return errors.Wrap(err, "could not create resource")
		}
		return nil
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get resource")
		return errors.Wrap(err, "could not get resource")
	}
	if !update() {
		logger.Debug("resource is up-to-date")
		return nil
	}
	logger.Info("updating resource")
	if err := r.Update(context.Background(), observed); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update resource")
		return errors.Wrap(err, "could not update resource")
	}
	return nil
}

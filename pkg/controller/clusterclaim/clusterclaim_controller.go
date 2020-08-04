package clusterclaim

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = "clusterclaim"
	finalizer      = "hive.openshift.io/claim"
)

// Add creates a new ClusterClaim Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new ReconcileClusterClaim
func NewReconciler(mgr manager.Manager) *ReconcileClusterClaim {
	logger := log.WithField("controller", ControllerName)
	return &ReconcileClusterClaim{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		logger: logger,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcileClusterClaim) error {
	// Create a new controller
	c, err := controller.New("clusterclaim-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterClaim
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterClaim{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterClaim{}

// ReconcileClusterClaim reconciles a CLusterClaim object
type ReconcileClusterClaim struct {
	client.Client
	logger log.FieldLogger
}

// Reconcile reconciles a ClusterClaim.
func (r *ReconcileClusterClaim) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("clusterClaim", request.NamespacedName)

	logger.Infof("reconciling cluster claim")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

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

	cd := &hivev1.ClusterDeployment{}
	switch err := r.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); {
	case apierrors.IsNotFound(err):
		return r.reconcileForDeletedCluster(claim, logger)
	case err != nil:
		logger.Log(controllerutils.LogLevel(err), "error getting ClusterDeployment")
		return reconcile.Result{}, err
	}

	switch cd.Spec.ClusterPoolRef.ClaimName {
	case "":
		return r.reconcileForNewAssignment(claim, cd, logger)
	case claim.Name:
		return r.reconcileForExistingAssignment(claim, logger)
	default:
		return r.reconcileForAssignmentConflict(claim, logger)
	}
}

func (r *ReconcileClusterClaim) reconcileDeletedClaim(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(claim, finalizer) {
		return reconcile.Result{}, nil
	}

	if clusterName := claim.Spec.Namespace; clusterName != "" {
		logger := logger.WithField("cluster", clusterName)
		cd := &hivev1.ClusterDeployment{}
		switch err := r.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: clusterName}, cd); {
		case apierrors.IsNotFound(err), cd.DeletionTimestamp != nil:
			logger.Info("cluster has already been deleted")
		case err != nil:
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error get ClusterDeployment")
			return reconcile.Result{}, err
		case cd.Spec.ClusterPoolRef == nil, cd.Spec.ClusterPoolRef.Namespace != claim.Namespace, cd.Spec.ClusterPoolRef.ClaimName != claim.Name:
			logger.Info("assignment of cluster to claim was not completed")
		default:
			if err := r.Delete(context.Background(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "error deleting ClusterDeployment")
				return reconcile.Result{}, err
			}
		}
	} else {
		logger.Info("removing finalizer from claim that was never assigned a cluster")
	}

	controllerutils.DeleteFinalizer(claim, finalizer)
	if err := r.Update(context.Background(), claim); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer from ClusterClaim")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterClaim) reconcileForDeletedCluster(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("assigned cluster has been deleted")
	conds, changed := controllerutils.SetClusterClaimConditionWithChangeCheck(
		claim.Status.Conditions,
		hivev1.ClusterClaimClusterDeletedCondition,
		corev1.ConditionTrue,
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

func (r *ReconcileClusterClaim) reconcileForNewAssignment(claim *hivev1.ClusterClaim, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Info("cluster assigned to claim")
	cd.Spec.ClusterPoolRef.ClaimName = claim.Name
	if err := r.Update(context.Background(), cd); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not set claim for ClusterDeployment")
		return reconcile.Result{}, err
	}
	return r.reconcileForExistingAssignment(claim, logger)
}

func (r *ReconcileClusterClaim) reconcileForExistingAssignment(claim *hivev1.ClusterClaim, logger log.FieldLogger) (reconcile.Result, error) {
	logger.Debug("claim has existing cluster assignment")
	conds, changed := controllerutils.SetClusterClaimConditionWithChangeCheck(
		claim.Status.Conditions,
		hivev1.ClusterClaimPendingCondition,
		corev1.ConditionFalse,
		"ClusterClaimed",
		"Cluster claimed",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
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
	claim.Status.Conditions = controllerutils.SetClusterClaimCondition(
		claim.Status.Conditions,
		hivev1.ClusterClaimPendingCondition,
		corev1.ConditionTrue,
		"AssignmentConflict",
		"Assigned cluster was claimed by a different ClusterClaim",
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if err := r.Update(context.Background(), claim); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update status of ClusterClaim")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

package clusterversion

import (
	"context"
	"github.com/openshift/hive/pkg/constants"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
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
	controllerName = "clusterleasepool"

	// lease pool label
)

// Add creates a new ClusterLeasePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterLeasePool{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterleasepool-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterLeasePool
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO: watch ClusterDeployment, map to it's owning LeasePool (if any) and queue it up

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterLeasePool{}

// ReconcileClusterLeasePool reconciles a CLusterLeasePool object
type ReconcileClusterLeasePool struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state ClusterLeasePool, checks if we currently have enough ClusterDeployments waiting, and
// attempts to reach the desired state if not.
func (r *ReconcileClusterLeasePool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := log.WithFields(log.Fields{
		"clusterLeasePool": request.Name,
		"namespace":        request.Namespace,
		"controller":       controllerName,
	})

	logger.Info("reconciling cluster lease pool")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterLeasePool instance
	clp := &hivev1.ClusterLeasePool{}
	err := r.Get(context.TODO(), request.NamespacedName, clp)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	// If the lease pool is deleted, do not reconcile.
	if clp.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// TODO: do stuff.

	// List all ClusterDeployments with a matching lease pool label:
	leaseCDs := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), leaseCDs, client.MatchingLabels(map[string]string{constants.ClusterLeasePoolNameLabel: clp.Name})); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments for lease pool")
	}

	installing := 0
	deleting := 0
	for _, cd := range leaseCDs.Items {
		if cd.DeletionTimestamp != nil {
			deleting += 1
		} else if !cd.Spec.Installed {
			installing += 1
		}
	}
	logger.WithFields(log.Fields{
		"installing": installing,
		"deleting":   deleting,
		"total":      len(leaseCDs.Items),
		"ready":      len(leaseCDs.Items) - installing - deleting,
	}).Info("found clusters for lease pool")

	// If too many, delete some.
	// TODO: improve logic here, delete oldest, or delete still installing in favor of those that are ready
	if len(leaseCDs.Items)-deleting > clp.Spec.Size {
		if err := r.deleteExcessClusters(clp, leaseCDs, logger); err != nil {
			return reconcile.Result{}, err
		}
	} else if len(leaseCDs.Items)-deleting < clp.Spec.Size {
		// If too few, create new InstallConfig and ClusterDeployment.
		if err := r.createNewClusters(clp, leaseCDs, logger); err != nil {
			return reconcile.Result{}, err
		}
	}

	logger.Debug("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterLeasePool) createNewClusters(
	clp *hivev1.ClusterLeasePool,
	leaseCDs *hivev1.ClusterDeploymentList,
	logger log.FieldLogger) error {
	return nil
}

func (r *ReconcileClusterLeasePool) deleteExcessClusters(
	clp *hivev1.ClusterLeasePool,
	leaseCDs *hivev1.ClusterDeploymentList,
	logger log.FieldLogger) error {

	logger.Info("too many clusters, searching for some to delete")
	deletionsNeeded := len(leaseCD.Items) - deleting - clp.Spec.Size
	for _, cd := range leaseCDs.Items {
		cdLog := logger.WithField("cluster", fmd.Sprintf("%s/%s", cd.Namespace, cd.Name))
		if cd.DeletionTimestamp != nil {
			cdLog.WithFields(log.Fields{"cdName": cd.Name, "cdNamespace": cd.Namespace}).Debug("cluster already deleting")
			continue
		}
		cdLog.Info("deleting cluster deployment")
		if err := r.Client.Delete(context.Background(), &cd); err != nil {
			cdLog.WithError(err).Error("error deleting cluster deployment")
			return err
		}
		deletionsNeeded -= 1
		if deletionsNeeded == 0 {
			cdLog.Info("no more deletions required")
			continue
		}
	}
	return nil
}

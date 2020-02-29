package clusterleaserequest

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName = "clusterleaserequest"
)

// Add creates a new ClusterLeaseRequest Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
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
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterLeasePool{}}, &handler.EnqueueRequestForObject{})
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

// Reconcile attempts to fill ClusterLeaseRequests with a cluster from the ClusterLeasePool.
func (r *ReconcileClusterLeasePool) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := log.WithFields(log.Fields{
		"clusterLeaseRequest": request.Name,
		"controller":          controllerName,
	})

	logger.Infof("reconciling cluster lease request: %v", request.Name)
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterLeaseRequest
	clr := &hivev1.ClusterLeaseRequest{}
	err := r.Get(context.TODO(), request.NamespacedName, clr)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("request no longer exists")
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		logger.WithError(err).Error("error reading object, requeue request")
		return reconcile.Result{}, err
	}

	if clr.DeletionTimestamp != nil {
		// TODO: need to act here, should probably kill an associated cluster deployment.
		return reconcile.Result{}, nil
	}

	// Ensure the ClusterLeasePool exists.
	pool := &hivev1.ClusterLeasePool{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: clr.Spec.ClusterLeasePoolRef.Name}, pool)
	if err != nil {
		if errors.IsNotFound(err) {
			// Nothing we can do right now. Note that a pool being deleted will *not* delete all
			// currently leased clusters, as people may be working on them, and they are not considered
			// as being in the pool any longer.
			logger.WithField("pool", clr.Spec.ClusterLeasePoolRef.Name).Warn("ClusterLeasePoolRef does not exist")
			return reconcile.Result{}, nil
		}
		logger.WithError(err).Error("error reading object, requeue request")
		return reconcile.Result{}, err
	}

	if clr.Spec.ClusterDeploymentRef != nil {
		logger.WithField("cluster", clr.Spec.ClusterDeploymentRef).Info("request already filled, nothing to do")
		return reconcile.Result{}, err
	}

	// TODO: Ensure the user exists.

	// Handle an edge case where we may have previously selected a ClusterDeployment for this request by successfully
	// updating it's labels to remove it from the pool and assign it to this request, but were unable to write that state
	// to the request itself.
	clusterList := &hivev1.ClusterDeploymentList{}
	if err := r.Client.List(context.Background(), clusterList,
		client.MatchingLabels(map[string]string{constants.ClusterLeaseRequestNameLabel: clr.Name})); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments for lease request")
		return reconcile.Result{}, err
	}
	if len(clusterList.Items) > 0 {
		if len(clusterList.Items) > 1 {
			logger.Error("found multiple ClusterDeployments already assigned to request, unable to reconcile")
			return reconcile.Result{}, fmt.Errorf("found multiple ClusterDeployments already assigned to request, unable to reconcile")
		}
		logger.Warn("found ClusterDeployment already assigned to request, updating reference")
		c := clusterList.Items[0]
		clr.Spec.ClusterDeploymentRef = &corev1.ObjectReference{Namespace: c.Namespace, Name: c.Name}
		if err := r.Update(context.Background(), clr); err != nil {
			logger.WithError(err).Error("error updating ClusterLeaseRequest with cluster details")
			return reconcile.Result{}, err
		}
	}

	// List all clusters in our desired pool and see if any are available:
	if err := r.Client.List(context.Background(), clusterList,
		client.MatchingLabels(map[string]string{constants.ClusterLeasePoolNameLabel: pool.Name})); err != nil {
		logger.WithError(err).Error("error listing ClusterDeployments for lease pool")
		return reconcile.Result{}, err
	}
	logger.WithFields(log.Fields{
		"total": len(clusterList.Items),
	}).Info("found clusters for lease pool")

	// Choose a cluster, attempt to grab it by updating it's annotations to (a) take it out of the
	// pool, and (b) assign it to this request. Failure to write here could easily mean someone else
	// grabbed it concurrently, in which case we requeue and pick up a new pool.
	for _, cd := range clusterList.Items {
		if cd.DeletionTimestamp != nil {
			logger.WithField("cluster", cd.Name).Infof("skipping cluster that is deleting")
			continue
		} else if !cd.Spec.Installed {
			logger.WithField("cluster", cd.Name).Infof("skipping cluster that is provisioning")
			continue
		}
		cdLog := logger.WithField("cluster", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name))
		cdLog.Info("claiming ClusterDeployment for request")
		// TODO: select random cluster from those that are available?
		delete(cd.Labels, constants.ClusterLeasePoolNameLabel)
		cd.Labels[constants.ClusterLeaseRequestNameLabel] = clr.Name
		cdLog.WithField("labels", cd.Labels).Debug("new ClusterDeployment labels")
		if err := r.Client.Update(context.Background(), &cd); err != nil {
			cdLog.WithError(err).Error("error updating ClusterDeployment labels to claim for request")
			return reconcile.Result{}, err
		}
		clr.Spec.ClusterDeploymentRef = &corev1.ObjectReference{Namespace: cd.Namespace, Name: cd.Name}
		if err := r.Update(context.Background(), clr); err != nil {
			// This should be fixed on next reconcile by the block above.
			cdLog.WithError(err).Error("error updating ClusterLeaseRequest with cluster details")
			return reconcile.Result{}, err
		}
		break
	}

	logger.Debug("reconcile complete")
	return reconcile.Result{}, nil
}

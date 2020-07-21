package clusterpoolnamespace

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

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
)

const (
	ControllerName                                 = "clusterpoolnamespace"
	minimumLifetime                                = 5 * time.Minute
	durationBetweenDeletingClusterDeploymentChecks = 1 * time.Minute
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterPoolNamespace{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		logger: log.WithField("controller", ControllerName),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(
		fmt.Sprintf("%s-controller", ControllerName),
		mgr,
		controller.Options{
			Reconciler:              r,
			MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles(),
		},
	)
	if err != nil {
		return err
	}

	// Watch for changes to Namespaces
	if err := c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	cdMapFn := func(a handler.MapObject) []reconcile.Request {
		cd := a.Object.(*hivev1.ClusterDeployment)
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{Name: cd.Namespace},
		}}
	}
	if err := c.Watch(
		&source.Kind{Type: &hivev1.ClusterDeployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(cdMapFn),
		},
	); err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterPoolNamespace{}

// ReconcileClusterPoolNamespace reconciles a Namespace object for the purpose of reaping namespaces created for
// ClusterPool clusters after the clusters have been deleted.
type ReconcileClusterPoolNamespace struct {
	client.Client
	logger log.FieldLogger
}

// Reconcile deletes a Namespace if it no longer contains any ClusterDeployments.
func (r *ReconcileClusterPoolNamespace) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("namespace", request.Name)

	logger.Info("reconciling namespace")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the Namespace instance
	namespace := &corev1.Namespace{}
	switch err := r.Get(context.Background(), request.NamespacedName, namespace); {
	case apierrors.IsNotFound(err):
		return reconcile.Result{}, nil
	case err != nil:
		return reconcile.Result{}, err
	}

	// If the Namespace is deleted, do not reconcile.
	if namespace.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// If the namespace was not created for a ClusterPool cluster, ignore it
	if _, ok := namespace.Labels[constants.OriginClusterPoolNameLabel]; !ok {
		return reconcile.Result{}, nil
	}

	if lifetime := time.Since(namespace.CreationTimestamp.Time); lifetime < minimumLifetime {
		logger.WithField("lifetime", lifetime).Debug("namespace is not old enough to delete; waiting longer for ClusterDeployment to be created")
		return reconcile.Result{RequeueAfter: minimumLifetime - lifetime}, nil
	}

	cdList := &hivev1.ClusterDeploymentList{}
	if err := r.List(context.Background(), cdList, client.InNamespace(namespace.Name)); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not list ClusterDeployments")
		return reconcile.Result{}, err
	}

	if len(cdList.Items) == 0 {
		logger.Info("deleting namespace since it contains no ClusterDeployments")
		if err := r.Delete(context.Background(), namespace); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error deleting namespace")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// If all of the ClusterDeployments have been deleted, then we need to due a Requeue After since the watch will
	// not trigger when the ClusterDeployment is finally removed from storage.
	for _, cd := range cdList.Items {
		if cd.DeletionTimestamp == nil {
			logger.WithField("clusterDeployment", cd.Name).Debug("ClusterDeployment has not been deleted")
			return reconcile.Result{}, nil
		}
	}

	logger.Debug("all ClusterDeployments deleted; waiting longer for ClusterDeployments to be removed from storage")
	return reconcile.Result{RequeueAfter: durationBetweenDeletingClusterDeploymentChecks}, nil
}

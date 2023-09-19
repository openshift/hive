package clusterpool

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func (r *ReconcileClusterPool) watchClusterDeployments(mgr manager.Manager, c controller.Controller) error {
	h := &clusterDeploymentEventHandler{
		EventHandler: handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, a client.Object) []reconcile.Request {
				cpKey := clusterPoolKey(a.(*hivev1.ClusterDeployment))
				if cpKey == nil {
					return nil
				}
				return []reconcile.Request{{NamespacedName: *cpKey}}
			},
		),
		reconciler: r,
	}
	return c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(h, controllerutils.IsClusterDeploymentErrorUpdateEvent))
}

var _ handler.EventHandler = &clusterDeploymentEventHandler{}

type clusterDeploymentEventHandler struct {
	handler.EventHandler
	reconciler *ReconcileClusterPool
}

// Create implements handler.EventHandler
func (h *clusterDeploymentEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("ClusterDeployment created")
	h.trackClusterDeploymentAdd(e.Object)
	h.EventHandler.Create(context.TODO(), e, q)
}

// When a ClusterDeployment is created, update the expectations of the ClusterPool that owns the ClusterDeployment.
func (h *clusterDeploymentEventHandler) trackClusterDeploymentAdd(obj interface{}) {
	cd := obj.(*hivev1.ClusterDeployment)
	if cd.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}
	cpKey := clusterPoolKey(cd)
	if cpKey == nil {
		return
	}
	h.reconciler.expectations.CreationObserved(cpKey.String())
}

func clusterPoolKey(cd *hivev1.ClusterDeployment) *types.NamespacedName {
	if cd.Spec.ClusterPoolRef == nil {
		return nil
	}
	return &types.NamespacedName{Namespace: cd.Spec.ClusterPoolRef.Namespace, Name: cd.Spec.ClusterPoolRef.PoolName}
}

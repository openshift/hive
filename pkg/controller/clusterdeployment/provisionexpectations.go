package clusterdeployment

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

func (r *ReconcileClusterDeployment) watchClusterProvisions(c controller.Controller) error {
	handler := &clusterProvisionEventHandler{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &hivev1.ClusterDeployment{},
		},
		reconciler: r,
	}
	return c.Watch(&source.Kind{Type: &hivev1.ClusterProvision{}}, handler)
}

var _ handler.EventHandler = &clusterProvisionEventHandler{}

type clusterProvisionEventHandler struct {
	handler.EnqueueRequestForOwner
	reconciler *ReconcileClusterDeployment
}

// Create implements handler.EventHandler
func (h *clusterProvisionEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("ClusterProvision created")
	h.reconciler.trackClusterProvisionAdd(e.Object)
	h.EnqueueRequestForOwner.Create(e, q)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileClusterDeployment) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *hivev1.ClusterDeployment {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	cd := &hivev1.ClusterDeployment{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}, cd); err != nil {
		return nil
	}
	if cd.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return cd
}

// When a clusterprovision is created, update the expectations of the clusterdeployment that owns the clusterprovision.
func (r *ReconcileClusterDeployment) trackClusterProvisionAdd(obj interface{}) {
	provision := obj.(*hivev1.ClusterProvision)
	if provision.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(provision); controllerRef != nil {
		cd := r.resolveControllerRef(provision.Namespace, controllerRef)
		if cd == nil {
			return
		}
		cdKey := types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}.String()
		r.expectations.CreationObserved(cdKey)
	}
}

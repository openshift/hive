package clusterprovision

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func (r *ReconcileClusterProvision) watchJobs(mgr manager.Manager, c controller.Controller) error {
	handler := &jobEventHandler{
		EventHandler: handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterProvision{}, handler.OnlyControllerOwner()),
		reconciler:   r,
	}
	return c.Watch(source.Kind(mgr.GetCache(), &batchv1.Job{}), handler)
}

var _ handler.EventHandler = &jobEventHandler{}

type jobEventHandler struct {
	handler.EventHandler
	reconciler *ReconcileClusterProvision
}

// Create implements handler.EventHandler
func (h *jobEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("Job created")
	h.reconciler.trackJobAdd(e.Object)
	h.EventHandler.Create(ctx, e, q)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileClusterProvision) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *hivev1.ClusterProvision {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		return nil
	}
	provision := &hivev1.ClusterProvision{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}, provision); err != nil {
		return nil
	}
	if provision.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	return provision
}

// When a job is created, update the expectations of the clusterprovision that owns the job.
func (r *ReconcileClusterProvision) trackJobAdd(obj interface{}) {
	job := obj.(*batchv1.Job)
	if job.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(job); controllerRef != nil {
		provision := r.resolveControllerRef(job.Namespace, controllerRef)
		if provision == nil {
			return
		}
		provisionKey := types.NamespacedName{Namespace: provision.Namespace, Name: provision.Name}.String()
		r.expectations.CreationObserved(provisionKey)
	}
}

package clusterprovision

import (
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func (r *ReconcileClusterProvision) watchJobs(c controller.Controller) error {
	handler := &jobEventHandler{
		handler:    handler.EnqueueRequestsFromMapFunc(controllerutils.MapJobToOwner(&hivev1.ClusterProvision{}, r.logger)),
		reconciler: r,
	}
	return c.Watch(&source.Kind{Type: &batchv1.Job{}}, handler)
}

var _ handler.EventHandler = &jobEventHandler{}

type jobEventHandler struct {
	handler    handler.EventHandler
	reconciler *ReconcileClusterProvision
}

// Create implements handler.EventHandler
func (h *jobEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.trackJobAdd(e.Object)
	h.handler.Create(e, q)
}

// Update is called in response to an update event -  e.g. Pod Updated.
func (h *jobEventHandler) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	h.handler.Update(e, q)
}

// Delete is called in response to a delete event - e.g. Pod Deleted.
func (h *jobEventHandler) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	h.handler.Delete(e, q)
}

// Generic is called in response to an event of an unknown type or a synthetic event triggered as a cron or
// external trigger request - e.g. reconcile Autoscaling, or a Webhook.
func (h *jobEventHandler) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	h.handler.Generic(e, q)
}

// When a job is created, update the expectations of the clusterprovision that owns the job.
func (r *ReconcileClusterProvision) trackJobAdd(obj interface{}) {
	job := obj.(*batchv1.Job)
	if job.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}

	provisionKey := controllerutils.GetJobOwnerNSName(job, &hivev1.ClusterProvision{}, r.logger)
	if provisionKey == nil {
		return
	}
	r.logger.WithField("job", job.Name).Info("Job created")
	r.expectations.CreationObserved(provisionKey.String())
}

package remotemachineset

import (
	"context"
	log "github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func (r *ReconcileRemoteMachineSet) watchMachinePoolNameLeases(c controller.Controller) error {
	h := &machinePoolNameLeaseEventHandler{
		EnqueueRequestForOwner: handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &hivev1.MachinePool{},
		},
		reconciler: r,
	}
	return c.Watch(&source.Kind{Type: &hivev1.MachinePoolNameLease{}}, h)
}

var _ handler.EventHandler = &machinePoolNameLeaseEventHandler{}

type machinePoolNameLeaseEventHandler struct {
	handler.EnqueueRequestForOwner
	reconciler *ReconcileRemoteMachineSet
}

// Create implements handler.EventHandler
func (h *machinePoolNameLeaseEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("running Create handler for MachinePoolNameLease")
	h.reconciler.trackLeaseAdd(e.Object)
	h.EnqueueRequestForOwner.Create(e, q)
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileRemoteMachineSet) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *hivev1.MachinePool {
	// We can't look up by UID, so look up by Name and then verify UID.
	// Don't even try to look up by Name if it's the wrong Kind.
	if controllerRef.Kind != controllerKind.Kind {
		r.logger.WithFields(log.Fields{
			"controllerRefKind":  controllerRef.Kind,
			"controllerKindKind": controllerKind.Kind,
		}).Debug("controller ref kind does not match controller kind")
		return nil
	}
	r.logger.Debug("controller ref kind matches, checking UID")
	mp := &hivev1.MachinePool{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: controllerRef.Name}, mp); err != nil {
		r.logger.WithError(err).Errorf("error getting machine pool: %s", controllerRef.Name)
		return nil
	}
	if mp.UID != controllerRef.UID {
		// The controller we found with this Name is not the same one that the
		// ControllerRef points to.
		return nil
	}
	r.logger.Debug("found matching UID")
	return mp
}

// When a lease is created, update the expectations of the machinepool that owns the lease.
func (r *ReconcileRemoteMachineSet) trackLeaseAdd(obj interface{}) {
	r.logger.Debug("tracking lease add")
	lease := obj.(*hivev1.MachinePoolNameLease)
	if lease.DeletionTimestamp != nil {
		// on a restart of the controller, it's possible a new object shows up in a state that
		// is already pending deletion. Prevent the object from being a creation observation.
		return
	}

	// If it has a ControllerRef, that's all that matters.
	if controllerRef := metav1.GetControllerOf(lease); controllerRef != nil {
		pool := r.resolveControllerRef(lease.Namespace, controllerRef)
		if pool == nil {
			r.logger.Debug("no pool returned from resolveControllerRef")
			return
		}
		poolKey := types.NamespacedName{Namespace: pool.Namespace, Name: pool.Name}.String()
		r.logger.Info("creation observed")
		r.expectations.CreationObserved(poolKey)
	}
}

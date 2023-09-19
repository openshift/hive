package machinepool

import (
	"context"

	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func (r *ReconcileMachinePool) watchMachinePoolNameLeases(mgr manager.Manager, c controller.Controller) error {
	h := &machinePoolNameLeaseEventHandler{
		EventHandler: handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.MachinePool{}, handler.OnlyControllerOwner()),
		reconciler:   r,
	}
	return c.Watch(source.Kind(mgr.GetCache(), &hivev1.MachinePoolNameLease{}), h)
}

var _ handler.EventHandler = &machinePoolNameLeaseEventHandler{}

type machinePoolNameLeaseEventHandler struct {
	handler.EventHandler
	reconciler *ReconcileMachinePool
}

// Create implements handler.EventHandler
func (h *machinePoolNameLeaseEventHandler) Create(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
	h.reconciler.logger.Info("running Create handler for MachinePoolNameLease")
	h.reconciler.trackLeaseAdd(e.Object)
	h.EventHandler.Create(ctx, e, q)
}

// Delete implements handler.EventHandler
func (h *machinePoolNameLeaseEventHandler) Delete(ctx context.Context, e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	logger := h.reconciler.logger
	logger.Info("running Delete handler for MachinePoolNameLease, requeuing all pools for cluster")
	lease, ok := e.Object.(*hivev1.MachinePoolNameLease)
	if !ok {
		logger.Warn("Delete handler called for non-MachinePoolNameLease: %v", e.Object)
		return
	}
	if _, ok := lease.Labels[constants.ClusterDeploymentNameLabel]; !ok {
		logger.WithFields(log.Fields{
			"lease":     lease.Name,
			"namespace": lease.Namespace,
		}).Warnf("deleted lease has no %s label, unable to requeue all pools for cluster", constants.ClusterDeploymentNameLabel)
		return
	}
	logger.Info("listing all pools for cluster deployment")

	// If a lease is deleted, requeue all the pools for the cluster, somebody might be waiting for a lease to free up.
	// TODO: we do not currently label machine pools as belonging to the cluster, when we do we could use this to filter
	// during the query itself.
	clusterMachinePools := &hivev1.MachinePoolList{}
	err := h.reconciler.List(context.TODO(), clusterMachinePools, client.InNamespace(lease.Namespace))
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err),
			"unable to list machine pools for cluster")
		return
	}
	logger.Debugf("found %d MachinePools for cluster", len(clusterMachinePools.Items))
	for _, mp := range clusterMachinePools.Items {
		if mp.Spec.ClusterDeploymentRef.Name != lease.Labels[constants.ClusterDeploymentNameLabel] {
			continue
		}
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      mp.Name,
			Namespace: mp.Namespace,
		}})
	}
}

// resolveControllerRef returns the controller referenced by a ControllerRef,
// or nil if the ControllerRef could not be resolved to a matching controller
// of the correct Kind.
func (r *ReconcileMachinePool) resolveControllerRef(namespace string, controllerRef *metav1.OwnerReference) *hivev1.MachinePool {
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
func (r *ReconcileMachinePool) trackLeaseAdd(obj interface{}) {
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

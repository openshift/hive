package utils

import (
	"context"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func EnqueueDNSZonesOwnedByClusterDeployment(c client.Client, logger log.FieldLogger) handler.EventHandler {

	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, mapObj client.Object) []reconcile.Request {
		dnsZones := &hivev1.DNSZoneList{}
		if err := c.List(
			context.TODO(),
			dnsZones,
			client.InNamespace(mapObj.GetNamespace()),
			client.MatchingLabels{
				constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
				constants.ClusterDeploymentNameLabel: mapObj.GetName(),
			},
		); err != nil {
			logger.WithError(err).Log(LogLevel(err), "could not list DNS zones owned by ClusterDeployment")
			return nil
		}
		requests := make([]reconcile.Request, len(dnsZones.Items))
		for i, dnsZone := range dnsZones.Items {
			requests[i] = reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&dnsZone)}
		}
		return requests
	})
}

// ReconcileDNSZoneForRelocation performs reconciliation on a DNSZone that is in the midst of a relocation to a new
// Hive instance.
// If the DNSZone is undergoing relocation, then the source Hive instance should not act on the DNSZone.
// If the DNSZone is undergoing relocation, then the destination Hive instance should not act on the DNSZone except to
// allow for a delete.
// If the DSNZone has completed relocation, then the source Hive instance should not act on the DNSZone except to remove
// the finalizer.
func ReconcileDNSZoneForRelocation(c client.Client, logger log.FieldLogger, dnsZone *hivev1.DNSZone, finalizer string) (*reconcile.Result, error) {
	_, relocateStatus, err := IsRelocating(dnsZone)
	// Block reconciliation when relocate status cannot be determined.
	if err != nil {
		logger.WithField("annotation", constants.RelocateAnnotation).WithError(err).Error("could not determine whether DNSZone is relocating")
		return nil, errors.Wrap(err, "could not determine whether DNSZone is relocating")
	}
	switch relocateStatus {
	// Allow reconciliation when not relocating.
	case "":
		logger.Debug("DNSZone is not involved in a relocate")
		return nil, nil
	// Block reconciliation when relocating out to another cluster.
	case hivev1.RelocateOutgoing:
		logger.Info("reconciling DNSZone is disabled for outgoing relocate")
		return &reconcile.Result{}, nil
	// Block reconciliation when relocating in from another cluster. Remove the finalizer if the DNSZone has been
	// deleted. This allows the DNSZone copied to the destination cluster to be deleted and replaced or to be deleted
	// after a failed reconciliation.
	case hivev1.RelocateIncoming:
		logger.Info("reconciling DNSZone is disabled for incoming relocate")
		if dnsZone.DeletionTimestamp != nil {
			if err := removeFinalizerIfPresent(c, logger, dnsZone, finalizer); err != nil {
				return nil, err
			}
		}
		return &reconcile.Result{}, nil
	// Clear finalizer on a DNSZone that has completed relocation out to another cluster.
	case hivev1.RelocateComplete:
		logger.Info("reconciling DNSZone is disabled after being relocated")
		return &reconcile.Result{}, removeFinalizerIfPresent(c, logger, dnsZone, finalizer)
	default:
		logger.WithField("annotation", constants.RelocateAnnotation).Error("unknown relocate status")
		return nil, errors.New("unknown relocate status")
	}
}

func removeFinalizerIfPresent(c client.Client, logger log.FieldLogger, obj hivev1.MetaRuntimeObject, finalizer string) error {
	if !HasFinalizer(obj, finalizer) {
		return nil
	}
	logger = logger.WithField("finalizer", finalizer)
	logger.Debug("Removing finalizer")
	DeleteFinalizer(obj, finalizer)
	if err := c.Update(context.TODO(), obj); err != nil {
		logger.WithError(err).Log(LogLevel(err), "failed to remove finalizer")
		return errors.Wrap(err, "failed to remove finalizer")
	}
	return nil
}

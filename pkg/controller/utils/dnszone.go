package utils

import (
	"context"

	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func EnqueueDNSZonesOwnedByClusterDeployment(c client.Client, logger log.FieldLogger) handler.EventHandler {
	return &handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(func(mapObj handler.MapObject) []reconcile.Request {
		dnsZones := &hivev1.DNSZoneList{}
		if err := c.List(
			context.TODO(),
			dnsZones,
			client.InNamespace(mapObj.Meta.GetNamespace()),
			client.MatchingLabels{
				constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
				constants.ClusterDeploymentNameLabel: mapObj.Meta.GetName(),
			},
		); err != nil {
			logger.WithError(err).Log(LogLevel(err), "could not list DNS zones owned by ClusterDeployment")
			return nil
		}
		requests := make([]reconcile.Request, len(dnsZones.Items))
		for i, dnsZone := range dnsZones.Items {
			request, err := client.ObjectKeyFromObject(&dnsZone)
			if err != nil {
				logger.WithError(err).Error("could not get object key for DNS zone")
				continue
			}
			requests[i] = reconcile.Request{NamespacedName: request}
		}
		return requests
	})}
}

// ReconcileDNSZoneForRelocation performs reconciliation on a DNSZone owned by a ClusterDeployment that is either in the midst
// of a relocation or has been relocated to a new Hive instance.
//
// Wait for owning ClusterDeployment to exist before reconciling DNSZone. This is necessary for cluster relocation.
//
// If the owning ClusterDeployment is undergoing relocation, then the source cluster should not act on the DNSZone.
//
// If the owning ClusterDeployment has completed relocation, then the source cluster should not act on the DNSZone
// except to remove the finalizer.
//
// The DNSZone is placed in the destination cluster before the ClusterDeployment is. In the event of an aborted
// relocation, the destination cluster should not act on the DNSZone.
func ReconcileDNSZoneForRelocation(c client.Client, logger log.FieldLogger, dnsZone *hivev1.DNSZone, finalizer string) (*reconcile.Result, error) {
	if dnsZone.Labels[constants.DNSZoneTypeLabel] != constants.DNSZoneTypeChild {
		return nil, nil
	}
	cdName, ownedByCD := dnsZone.Labels[constants.ClusterDeploymentNameLabel]
	if !ownedByCD {
		return nil, nil
	}
	logger = logger.WithField("clusterDeployment", cdName)
	cd := &hivev1.ClusterDeployment{}
	switch err := c.Get(context.TODO(), client.ObjectKey{Namespace: dnsZone.Namespace, Name: cdName}, cd); {
	case apierrors.IsNotFound(err):
		logger.Info("owning ClusterDeployment not found")
		return &reconcile.Result{}, nil
	case err != nil:
		logger.WithError(err).Log(LogLevel(err), "could not get owning ClusterDeployment")
		return &reconcile.Result{}, err
	}
	if _, relocating := cd.Annotations[constants.RelocatingAnnotation]; relocating {
		logger.WithField("annotation", constants.RelocatingAnnotation).Info("reconciling DNSZone is disabled by annotation")
		return &reconcile.Result{}, nil
	}
	if _, relocated := cd.Annotations[constants.RelocatedAnnotation]; relocated {
		logger.WithField("annotation", constants.RelocatedAnnotation).Info("reconciling DNSZone is disabled by annotation")
		if HasFinalizer(dnsZone, finalizer) {
			logger := logger.WithField("finalizer", finalizer)
			logger.Debug("Removing finalizer")
			DeleteFinalizer(dnsZone, finalizer)
			if err := c.Update(context.TODO(), dnsZone); err != nil {
				logger.WithError(err).Log(LogLevel(err), "Failed to remove finalizer")
				return &reconcile.Result{}, err
			}
		}
		return &reconcile.Result{}, nil
	}
	return nil, nil
}

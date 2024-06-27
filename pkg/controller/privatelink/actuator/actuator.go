package actuator

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/privatelink/dnsrecord"
)

type Actuator interface {

	// Cleanup cleans up the cloud resources.
	Cleanup(*hivev1.ClusterDeployment, *hivev1.ClusterMetadata, log.FieldLogger) error

	// CleanupRequired returns true if there are resources to be cleaned up.
	CleanupRequired(*hivev1.ClusterDeployment) bool

	// Reconcile reconciles the required resources.
	Reconcile(*hivev1.ClusterDeployment, *hivev1.ClusterMetadata, *dnsrecord.DnsRecord, log.FieldLogger) (reconcile.Result, error)

	// ShouldSync returns true if there are changes that need to be made.
	ShouldSync(*hivev1.ClusterDeployment) bool

	// Validate validates a cluster deployment.
	Validate(*hivev1.ClusterDeployment, log.FieldLogger) error
}

type ActuatorType string

const (
	ActuatorTypeHub  ActuatorType = "Hub"
	ActuatorTypeLink ActuatorType = "Link"
)

package awsactuator

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/dnsrecord"
)

// Ensure AWSHubActuator implements the Actuator interface. This will fail at compile time when false.
var _ actuator.Actuator = &AWSHubActuator{}

type AWSHubActuator struct {
}

func NewAWSHubActuator(client *client.Client, config *hivev1.AWSPrivateLinkConfig, logger log.FieldLogger) (*AWSHubActuator, error) {
	return &AWSHubActuator{}, nil
}

// Cleanup cleans up the cloud resources.
func (a *AWSHubActuator) Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger log.FieldLogger) error {
	return nil
}

// CleanupRequired returns true if there are resources to be cleaned up.
func (a *AWSHubActuator) CleanupRequired(cd *hivev1.ClusterDeployment) bool {
	return false
}

// Reconcile reconciles the required resources.
func (a *AWSHubActuator) Reconcile(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, dnsRecord *dnsrecord.DnsRecord, logger log.FieldLogger) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// ShouldSync returns true if there are changes that need to be made.
func (a *AWSHubActuator) ShouldSync(cd *hivev1.ClusterDeployment) bool {
	return false
}

// Validate validates a cluster deployment.
func (a *AWSHubActuator) Validate(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	return nil
}

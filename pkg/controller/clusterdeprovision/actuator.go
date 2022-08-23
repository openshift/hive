package clusterdeprovision

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Actuator interface is the interface that is used to add cloud provider support to the deprovision controller.
type Actuator interface {
	// CanHandle returns true if the actuator can handle a particular ClusterDeprovision
	CanHandle(cd *hivev1.ClusterDeprovision) bool

	// TestCredentials returns nil if the credential check succeeds. Otherwise returns the error.
	TestCredentials(clusterDeprovision *hivev1.ClusterDeprovision, dpClient, cpClient client.Client, logger log.FieldLogger) error
}

package installmanager

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LogUploaderActuator interface is the interface that is used to add provider support for uploading logs.
type LogUploaderActuator interface {
	// IsConfigured returns true if the actuator can handle a particular case
	IsConfigured() bool

	// UploadLogs uploads installer logs to the provider's storage mechanism.
	UploadLogs(clusterName string, clusterprovision *hivev1.ClusterProvision, c client.Client, log log.FieldLogger, filenames ...string) error
}

package hibernation

//go:generate mockgen -source=./hibernation_actuator.go -destination=./mock/hibernation_actuator_generated.go -package=mock

import (
	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// HibernationActuator is the interface that the hibernation controller uses to
// interact with cloud providers.
type HibernationActuator interface {
	// CanHandle returns true if the actuator can handle a particular ClusterDeployment
	CanHandle(cd *hivev1.ClusterDeployment) bool
	// StopMachines will start machines belonging to the given ClusterDeployment
	StopMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error
	// StartMachines will select machines belonging to the given ClusterDeployment
	StartMachines(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) error
	// MachinesRunning will return true if the machines associated with the given
	// ClusterDeployment are in a running state. It also returns a list of machines that
	// are not running.
	MachinesRunning(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error)
	// MachinesStopped will return true if the machines associated with the given
	// ClusterDeployment are in a stopped state. it also returns a list of machines
	// that have not stopped.
	MachinesStopped(cd *hivev1.ClusterDeployment, hiveClient client.Client, logger log.FieldLogger) (bool, []string, error)
}

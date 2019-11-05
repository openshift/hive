package syncmachineset

import (
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	installtypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

//go:generate mockgen -source=./actuator.go -destination=./mock/actuator_generated.go -package=mock

// Actuator is the interface that must be implemented to standardize generating
// and returning the list of MachineSets to by synced to the remote cluster.
type Actuator interface {
	GenerateMachineSets(*hivev1.ClusterDeployment, *installtypes.InstallConfig, log.FieldLogger) ([]*machineapi.MachineSet, error)
}

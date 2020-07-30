package remotemachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installovirt "github.com/openshift/installer/pkg/asset/machines/ovirt"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesovirt "github.com/openshift/installer/pkg/types/ovirt"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// OvirtActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type OvirtActuator struct {
	logger log.FieldLogger
}

var _ Actuator = &OvirtActuator{}

// NewOvirtActuator is the constructor for building a OvirtActuator
func NewOvirtActuator(logger log.FieldLogger) (*OvirtActuator, error) {
	actuator := &OvirtActuator{
		logger: logger,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *OvirtActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.Ovirt == nil {
		return nil, false, errors.New("ClusterDeployment is not for oVirt")
	}
	if pool.Spec.Platform.Ovirt == nil {
		return nil, false, errors.New("MachinePool is not for oVirt")
	}

	computePool := baseMachinePool(pool)

	computePool.Platform.Ovirt = &installertypesovirt.MachinePool{
		CPU: &installertypesovirt.CPU{
			Cores:   pool.Spec.Platform.Ovirt.CPU.Cores,
			Sockets: pool.Spec.Platform.Ovirt.CPU.Sockets,
		},
		MemoryMB: pool.Spec.Platform.Ovirt.MemoryMB,
		OSDisk: &installertypesovirt.Disk{
			SizeGB: pool.Spec.Platform.Ovirt.OSDisk.SizeGB,
		},
		VMType: installertypesovirt.VMType(pool.Spec.Platform.Ovirt.VMType),
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			Ovirt: &installertypesovirt.Platform{
				ClusterID:       cd.Spec.Platform.Ovirt.ClusterID,
				StorageDomainID: cd.Spec.Platform.Ovirt.StorageDomainID,
				NetworkName:     cd.Spec.Platform.Ovirt.NetworkName,
			},
		},
	}

	// The installer will upload a copy of the RHCOS image for the release with this name.
	// It is possible to override this in the installer with the OPENSHIFT_INSTALL_OS_IMAGE_OVERRIDE
	// env var, but we do not expose this in Hive, so it should be safe to assume the installers default
	// name format. If we do add support for uploading images more efficiently (as it's a 2GB dl+ul per
	// cluster install), we should stick to this same name format, or update this line of code.
	osImage := fmt.Sprintf("%s-rhcos", cd.Spec.ClusterMetadata.InfraID)

	installerMachineSets, err := installovirt.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, computePool, osImage, workerRole, workerUserData)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

package remotemachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installosp "github.com/openshift/installer/pkg/asset/machines/openstack"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesosp "github.com/openshift/installer/pkg/types/openstack"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// OpenStackActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type OpenStackActuator struct {
	logger log.FieldLogger
}

var _ Actuator = &OpenStackActuator{}

// NewOpenStackActuator is the constructor for building a OpenStackActuator
func NewOpenStackActuator(logger log.FieldLogger) (*OpenStackActuator, error) {
	actuator := &OpenStackActuator{
		logger: logger,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *OpenStackActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.OpenStack == nil {
		return nil, false, errors.New("ClusterDeployment is not for OpenStack")
	}
	if pool.Spec.Platform.OpenStack == nil {
		return nil, false, errors.New("MachinePool is not for OpenStack")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.OpenStack = &installertypesosp.MachinePool{
		FlavorName: pool.Spec.Platform.OpenStack.Flavor,
	}

	if pool.Spec.Platform.OpenStack.RootVolume != nil {
		computePool.Platform.OpenStack.RootVolume = &installertypesosp.RootVolume{
			Size: pool.Spec.Platform.OpenStack.RootVolume.Size,
			Type: pool.Spec.Platform.OpenStack.RootVolume.Type,
		}
	}

	// I do not know why the installer chose to represent this as a "0" or "1" string,
	// but the resulting MachineSets use a bool, so we're opting to model this more correctly
	// in our API and save a little validation.
	trunkSupportStr := "0"
	if cd.Spec.Platform.OpenStack.TrunkSupport {
		trunkSupportStr = "1"
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			OpenStack: &installertypesosp.Platform{
				TrunkSupport: trunkSupportStr,
			},
		},
	}

	// The installer will assume to upload a copy of the RHCOS image for the release with this name.
	// It is possible to override this in the installer with the OPENSHIFT_INSTALL_OS_IMAGE_OVERRIDE
	// env var, but we do not expose this in Hive, so it should be safe to assume the installers default
	// name format. If we do add support for uploading images more efficiently (as it's a 2GB dl+ul per
	// cluster install), we should stick to this same name format, or update this line of code.
	osImage := fmt.Sprintf("%s-rhcos", cd.Spec.ClusterMetadata.InfraID)

	installerMachineSets, err := installosp.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, computePool, osImage, workerRole, workerUserData)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

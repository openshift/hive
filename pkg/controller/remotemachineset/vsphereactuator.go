package remotemachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installvsphere "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesvsphere "github.com/openshift/installer/pkg/types/vsphere"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// VSphereActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type VSphereActuator struct {
	logger log.FieldLogger
}

var _ Actuator = &VSphereActuator{}

// NewVSphereActuator is the constructor for building a VSphereActuator
func NewVSphereActuator(logger log.FieldLogger) (*VSphereActuator, error) {
	actuator := &VSphereActuator{
		logger: logger,
	}
	return actuator, nil
}

// GenerateMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *VSphereActuator) GenerateMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
	if cd.Spec.ClusterMetadata == nil {
		return nil, false, errors.New("ClusterDeployment does not have cluster metadata")
	}
	if cd.Spec.Platform.VSphere == nil {
		return nil, false, errors.New("ClusterDeployment is not for VSphere")
	}
	if pool.Spec.Platform.VSphere == nil {
		return nil, false, errors.New("MachinePool is not for VSphere")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.VSphere = &installertypesvsphere.MachinePool{
		NumCPUs:           pool.Spec.Platform.VSphere.NumCPUs,
		NumCoresPerSocket: pool.Spec.Platform.VSphere.NumCoresPerSocket,
		MemoryMiB:         pool.Spec.Platform.VSphere.MemoryMiB,
		OSDisk: installertypesvsphere.OSDisk{
			DiskSizeGB: pool.Spec.Platform.VSphere.DiskSizeGB,
		},
	}

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: &installertypesvsphere.Platform{
				VCenter:          cd.Spec.Platform.VSphere.VCenter,
				Datacenter:       cd.Spec.Platform.VSphere.Datacenter,
				DefaultDatastore: cd.Spec.Platform.VSphere.DefaultDatastore,
				Folder:           cd.Spec.Platform.VSphere.Folder,
				Cluster:          cd.Spec.Platform.VSphere.Cluster,
				Network:          cd.Spec.Platform.VSphere.Network,
			},
		},
	}

	// The installer will assume to upload a copy of the RHCOS image for the release with this name.
	// It is possible to override this in the installer with the OPENSHIFT_INSTALL_OS_IMAGE_OVERRIDE
	// env var, but we do not expose this in Hive, so it should be safe to assume the installers default
	// name format. If we do add support for uploading images more efficiently (as it's a 2GB dl+ul per
	// cluster install), we should stick to this same name format, or update this line of code.
	osImage := fmt.Sprintf("%s-rhcos", cd.Spec.ClusterMetadata.InfraID)

	installerMachineSets, err := installvsphere.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, computePool, osImage, workerRole, workerUserData)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

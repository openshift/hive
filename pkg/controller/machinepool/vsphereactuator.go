package machinepool

import (
	"github.com/openshift/hive/pkg/clusterresource"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installvsphere "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesvsphere "github.com/openshift/installer/pkg/types/vsphere"
	vsphereutil "github.com/openshift/machine-api-operator/pkg/controller/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// VSphereActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type VSphereActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &VSphereActuator{}

// NewVSphereActuator is the constructor for building a VSphereActuator
func NewVSphereActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (*VSphereActuator, error) {
	osImage, err := getVSphereOSImage(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting os image from master machine")
		return nil, err
	}
	actuator := &VSphereActuator{
		logger:  logger,
		osImage: osImage,
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
	if cd.Spec.Platform.VSphere.VSphere == nil {
		return nil, false, errors.New("VSphere CD with deprecated fields has not been updated by CD controller yet, requeueing...")
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
	//
	// HACK: we cannot expose installer types from within the hive API, so we do a round-trip
	// through the vsphere cloudbuilder, which knows how to construct an install config
	// from an openshift api configv1.VSpherePlatformSpec
	ic := &installertypes.InstallConfig{}
	cloudBuilder := clusterresource.NewDummyVSphereCloudBuilder()
	cloudBuilder.VSphere = cd.Spec.Platform.VSphere.VSphere
	cloudBuilder.AttachToInstallConfig(ic)
	for _, failureDomain := range ic.VSphere.FailureDomains {
		if failureDomain.Topology.Template == "" {
			failureDomain.Topology.Template = a.osImage
		}
		if failureDomain.Topology.ResourcePool == "" {
			failureDomain.Topology.ResourcePool = pool.Spec.Platform.VSphere.ResourcePool
		}
	}

	installerMachineSets, err := installvsphere.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

// Get the OS image from an existing master machine.
func getVSphereOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := vsphereutil.ProviderSpecFromRawExtension(masterMachine.Spec.ProviderSpec.Value)
	if err != nil {
		logger.WithError(err).Warn("cannot decode VSphereMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode VSphereMachineProviderSpec from master machine")
	}
	osImage := providerSpec.Template
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

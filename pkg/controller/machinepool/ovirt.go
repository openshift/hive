package machinepool

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"
	ovirtproviderv1beta1 "github.com/openshift/cluster-api-provider-ovirt/pkg/apis/ovirtprovider/v1beta1"
	installovirt "github.com/openshift/installer/pkg/asset/machines/ovirt"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesovirt "github.com/openshift/installer/pkg/types/ovirt"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	msop "github.com/openshift/hive/pkg/controller/machinesetwithopflags"
)

// OvirtActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type OvirtActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &OvirtActuator{}

// NewOvirtActuator is the constructor for building a OvirtActuator
func NewOvirtActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (*OvirtActuator, error) {
	osImage, err := getOvirtOSImage(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting os image from master machine")
		return nil, err
	}
	actuator := &OvirtActuator{
		logger:  logger,
		osImage: osImage,
	}
	return actuator, nil
}

func (a *OvirtActuator) GetRemoteMachineSetsWithOpFlags(pool *hivev1.MachinePool, remoteMachineSets *machineapi.MachineSetList, infrastructure *configv1.Infrastructure, logger log.FieldLogger) ([]msop.MachineSetWithOpFlags, error) {
	remotes_to_update := []msop.MachineSetWithOpFlags{}
	for _, rMS := range remoteMachineSets.Items {
		remotes_to_update = append(remotes_to_update, msop.MachineSetWithOpFlags{MS: &rMS, NeedsUpdate: false, NeedsDelete: false})
	}
	return remotes_to_update, nil
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

	computePool.Platform.Ovirt = &installertypesovirt.MachinePool{}

	if cpu := pool.Spec.Platform.Ovirt.CPU; cpu != nil {
		computePool.Platform.Ovirt.CPU = &installertypesovirt.CPU{
			Cores:   cpu.Cores,
			Sockets: cpu.Sockets,
		}
	}

	if pool.Spec.Platform.Ovirt.MemoryMB != int32(0) {
		computePool.Platform.Ovirt.MemoryMB = pool.Spec.Platform.Ovirt.MemoryMB
	}

	if disk := pool.Spec.Platform.Ovirt.OSDisk; disk != nil {
		computePool.Platform.Ovirt.OSDisk = &installertypesovirt.Disk{
			SizeGB: disk.SizeGB,
		}
	}

	if vmType := pool.Spec.Platform.Ovirt.VMType; vmType != "" {
		computePool.Platform.Ovirt.VMType = installertypesovirt.VMType(vmType)
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

	installerMachineSets, err := installovirt.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		a.osImage,
		workerRole,
		workerUserDataName,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

// Get the OS image from an existing master machine.
func getOvirtOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeOvirtMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
	if err != nil {
		logger.WithError(err).Warn("cannot decode OvirtMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode OvirtMachineProviderSpec from master machine")
	}
	osImage := providerSpec.TemplateName
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

func decodeOvirtMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*ovirtproviderv1beta1.OvirtMachineProviderSpec, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(ovirtproviderv1beta1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode Ovirt ProviderSpec: %v", err)
	}
	spec, ok := obj.(*ovirtproviderv1beta1.OvirtMachineProviderSpec)
	if !ok {
		return nil, fmt.Errorf("unexpected object: %#v", gvk)
	}
	return spec, nil
}

package machinepool

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ovirtprovider "github.com/openshift/cluster-api-provider-ovirt/pkg/apis"
	ovirtproviderv1beta1 "github.com/openshift/cluster-api-provider-ovirt/pkg/apis/ovirtprovider/v1beta1"
	installovirt "github.com/openshift/installer/pkg/asset/machines/ovirt"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesovirt "github.com/openshift/installer/pkg/types/ovirt"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// OvirtActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type OvirtActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &OvirtActuator{}

func addOvirtProviderToScheme(scheme *runtime.Scheme) error {
	return ovirtprovider.AddToScheme(scheme)
}

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

// GenerateMAPIMachineSets satisfies the Actuator interface and will take a clusterDeployment and return a list of MachineSets
// to sync to the remote cluster.
func (a *OvirtActuator) GenerateMAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*machineapi.MachineSet, bool, error) {
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
	installerMachineSets = preserveOvirtMachineSetNameSuffix(installerMachineSets)

	return installerMachineSets, true, nil
}

// GenerateCAPIMachineSets takes a clusterDeployment and returns a list of upstream CAPI MachineSets
func (a *OvirtActuator) GenerateCAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*capiv1.MachineSet, []client.Object, bool, error) {
	return nil, nil, false, errors.New("GenerateCAPIMachineSets is not implemented for provider")
}

func (a *OvirtActuator) GetLocalMachineTemplates(lc client.Client, targetNamespace string, logger log.FieldLogger) ([]client.Object, error) {
	return nil, errors.New("GetLocalMachineTemplates is not implemented for provider")
}

// preserveOvirtMachineSetNameSuffix ensures that machineset names have a "-0" suffix. The suffix was
// removed from instalovirt.MachineSets so we maintain it here to prevent machineset replacement.
func preserveOvirtMachineSetNameSuffix(machineSets []*machineapi.MachineSet) []*machineapi.MachineSet {
	for _, ms := range machineSets {
		if !strings.HasSuffix(ms.Name, "-0") {
			ms.Name = fmt.Sprintf("%s-0", ms.Name)
		}
	}
	return machineSets
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
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

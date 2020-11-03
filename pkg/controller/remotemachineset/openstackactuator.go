package remotemachineset

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	openstackprovider "sigs.k8s.io/cluster-api-provider-openstack/pkg/apis"
	openstackproviderv1alpha1 "sigs.k8s.io/cluster-api-provider-openstack/pkg/apis/openstackproviderconfig/v1alpha1"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installosp "github.com/openshift/installer/pkg/asset/machines/openstack"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesosp "github.com/openshift/installer/pkg/types/openstack"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// OpenStackActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster.
type OpenStackActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &OpenStackActuator{}

func addOpenStackProviderToScheme(scheme *runtime.Scheme) error {
	return openstackprovider.AddToScheme(scheme)
}

// NewOpenStackActuator is the constructor for building a OpenStackActuator
func NewOpenStackActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (*OpenStackActuator, error) {
	osImage, err := getOpenStackOSImage(masterMachine, scheme, logger)
	if err != nil {
		logger.WithError(err).Error("error getting os image from master machine")
		return nil, err
	}
	actuator := &OpenStackActuator{
		logger:  logger,
		osImage: osImage,
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
	clusterVersion, err := getClusterVersion(cd)
	if err != nil {
		return nil, false, fmt.Errorf("Unable to get cluster version: %v", err)
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

	// Fake an install config as we do with other actuators. We only populate what we know is needed today.
	// WARNING: changes to use more of installconfig in the MachineSets function can break here. Hopefully
	// will be caught by unit tests.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			OpenStack: &installertypesosp.Platform{
				Cloud: cd.Spec.Platform.OpenStack.Cloud,
			},
		},
	}

	installerMachineSets, err := installosp.MachineSets(
		cd.Spec.ClusterMetadata.InfraID,
		ic,
		computePool,
		a.osImage,
		workerRole,
		workerUserData(clusterVersion),
		nil,
	)
	if err != nil {
		return nil, false, errors.Wrap(err, "failed to generate machinesets")
	}

	return installerMachineSets, true, nil
}

// Get the OS image from an existing master machine.
func getOpenStackOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeOpenStackMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
	if err != nil {
		logger.WithError(err).Warn("cannot decode OpenstackProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode OpenstackProviderSpec from master machine")
	}
	var osImage string
	if providerSpec.RootVolume != nil {
		osImage = providerSpec.RootVolume.SourceUUID
	} else {
		osImage = providerSpec.Image
	}
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

func decodeOpenStackMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*openstackproviderv1alpha1.OpenstackProviderSpec, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(openstackproviderv1alpha1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode OpenStack ProviderSpec: %v", err)
	}
	spec, ok := obj.(*openstackproviderv1alpha1.OpenstackProviderSpec)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

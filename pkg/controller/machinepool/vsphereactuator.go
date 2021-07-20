package machinepool

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	installvsphere "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installertypesvsphere "github.com/openshift/installer/pkg/types/vsphere"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	vsphereprovider "github.com/openshift/machine-api-operator/pkg/apis/vsphereprovider"
	vsphereproviderv1beta1 "github.com/openshift/machine-api-operator/pkg/apis/vsphereprovider/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// VSphereActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type VSphereActuator struct {
	logger  log.FieldLogger
	osImage string
}

var _ Actuator = &VSphereActuator{}

func addVSphereProviderToScheme(scheme *runtime.Scheme) error {
	return vsphereprovider.AddToScheme(scheme)
}

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

// GenerateCAPIMachineSets takes a clusterDeployment and returns a list of upstream CAPI MachineSets
func (a *VSphereActuator) GenerateCAPIMachineSets(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, logger log.FieldLogger) ([]*capiv1.MachineSet, bool, error) {
	machinesets := []*capiv1.MachineSet{}
	return machinesets, true, nil
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

	installerMachineSets, err := installvsphere.MachineSets(
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
func getVSphereOSImage(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (string, error) {
	providerSpec, err := decodeVSphereMachineProviderSpec(masterMachine.Spec.ProviderSpec.Value, scheme)
	if err != nil {
		logger.WithError(err).Warn("cannot decode VSphereMachineProviderSpec from master machine")
		return "", errors.Wrap(err, "cannot decode VSphereMachineProviderSpec from master machine")
	}
	osImage := providerSpec.Template
	logger.WithField("image", osImage).Debug("resolved image to use for new machinesets")
	return osImage, nil
}

func decodeVSphereMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*vsphereproviderv1beta1.VSphereMachineProviderSpec, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(vsphereproviderv1beta1.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode VSphere ProviderSpec: %v", err)
	}
	spec, ok := obj.(*vsphereproviderv1beta1.VSphereMachineProviderSpec)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

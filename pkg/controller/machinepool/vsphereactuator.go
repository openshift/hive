package machinepool

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installvspheremachines "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installvsphere "github.com/openshift/installer/pkg/types/vsphere"
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
	if cd.Spec.Platform.VSphere.Infrastructure == nil {
		return nil, false, errors.New("VSphere CD with deprecated fields has not been updated by CD controller yet, requeueing...")
	}
	if pool.Spec.Platform.VSphere == nil {
		return nil, false, errors.New("MachinePool is not for VSphere")
	}

	computePool := baseMachinePool(pool)
	computePool.Platform.VSphere = &pool.Spec.Platform.VSphere.MachinePool

	// Fake an install config as we do with other actuators.
	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: cd.Spec.Platform.VSphere.Infrastructure,
		},
	}
	for i := range ic.VSphere.FailureDomains {
		failureDomain := &ic.VSphere.FailureDomains[i] // because go ranges by copy, not by reference
		if a.osImage != "" {
			failureDomain.Topology.Template = a.osImage
		}
		if pool.Spec.Platform.VSphere.Topology != nil {
			newTopo, err := applyTopologyTemplate(failureDomain.Topology, *pool.Spec.Platform.VSphere.Topology, a.logger)
			if err != nil {
				return nil, false, err
			}

			failureDomain.Topology = newTopo
		}
		if pool.Spec.Platform.VSphere.DeprecatedResourcePool != "" {
			failureDomain.Topology.ResourcePool = pool.Spec.Platform.VSphere.DeprecatedResourcePool
		}
		if len(pool.Spec.Platform.VSphere.DeprecatedTagIDs) > 0 {
			failureDomain.Topology.TagIDs = pool.Spec.Platform.VSphere.DeprecatedTagIDs
		}
	}

	installerMachineSets, err := installvspheremachines.MachineSets(
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

func applyTopologyTemplate(base installvsphere.Topology, template installvsphere.Topology, logger log.FieldLogger) (out installvsphere.Topology, err error) {
	var ubase map[string]interface{}
	var utemplate map[string]interface{}

	ubase, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&base)
	if err != nil {
		return
	}

	utemplate, err = runtime.DefaultUnstructuredConverter.ToUnstructured(&template)
	if err != nil {
		return
	}

	for k, i := range utemplate {
		switch v := i.(type) {
		case string:
			if v != "" {
				ubase[k] = v
			}
		case []interface{}:
			switch v[0].(type) {
			case string:
				if len(v) > 0 {
					ubase[k] = v
				}
			default:
				logger.
					WithField("field-name", k).
					WithField("field-value", v).
					WithField("field-type", fmt.Sprintf("%T", v)).
					Warn("unexpected value on vsphere machinepool topology, please report this to the Hive maintainers")
			}
		default:
			logger.
				WithField("field-name", k).
				WithField("field-value", v).
				WithField("field-type", fmt.Sprintf("%T", v)).
				Warn("unexpected value on vsphere machinepool topology, please report this to the Hive maintainers")
		}
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(ubase, &out)
	return
}

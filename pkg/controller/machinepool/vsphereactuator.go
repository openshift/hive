package machinepool

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/runtime"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installvspheremachines "github.com/openshift/installer/pkg/asset/machines/vsphere"
	installertypes "github.com/openshift/installer/pkg/types"
	installervsphere "github.com/openshift/installer/pkg/types/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// VSphereActuator encapsulates the pieces necessary to be able to generate
// a list of MachineSets to sync to the remote cluster
type VSphereActuator struct {
	logger log.FieldLogger
}

var _ Actuator = &VSphereActuator{}

// NewVSphereActuator is the constructor for building a VSphereActuator
func NewVSphereActuator(masterMachine *machineapi.Machine, scheme *runtime.Scheme, logger log.FieldLogger) (*VSphereActuator, error) {
	return &VSphereActuator{
		logger: logger,
	}, nil
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

	ic := &installertypes.InstallConfig{
		Platform: installertypes.Platform{
			VSphere: cd.Spec.Platform.VSphere.Infrastructure.DeepCopy(),
		},
	}
	for i := range ic.VSphere.FailureDomains {
		failureDomain := &ic.VSphere.FailureDomains[i]
		if pool.Spec.Platform.VSphere.ResourcePool != "" {
			failureDomain.Topology.ResourcePool = pool.Spec.Platform.VSphere.ResourcePool
		}
		if len(pool.Spec.Platform.VSphere.TagIDs) > 0 {
			failureDomain.Topology.TagIDs = pool.Spec.Platform.VSphere.TagIDs
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

// backfillVSphereTemplates populates Topology.Template for each failure domain where it is
// empty by reading the template from a matching remote MachineSet. This handles clusters
// installed before the template naming convention change (HIVE-2391) where
// Infrastructure.DeepCopy() leaves Topology.Template empty, causing the installer to
// generate a new-format name that does not exist on the vCenter.
// poolResourcePool is the MachinePool-level ResourcePool override (may be empty).
func backfillVSphereTemplates(failureDomains []installervsphere.FailureDomain, remoteMachineSets []machineapi.MachineSet, infraID, poolResourcePool string, logger log.FieldLogger) {
	for i := range failureDomains {
		fd := &failureDomains[i]
		if fd.Topology.Template != "" {
			continue
		}
		template := findTemplateForFD(fd, remoteMachineSets, infraID, poolResourcePool, logger)
		if template != "" {
			fd.Topology.Template = template
			logger.WithFields(log.Fields{
				"failureDomain": fd.Name,
				"template":      template,
			}).Info("backfilled Topology.Template from remote MachineSet")
		}
	}
}

// findTemplateForFD searches remote MachineSets for one whose workspace fields match the
// given failure domain using a 5-field conjunction (MCO PR #5745 pattern):
// Datacenter + Datastore + Server + VMGroup + ResourcePool.
// poolResourcePool overrides fd.Topology.ResourcePool when the MachinePool specifies one.
func findTemplateForFD(fd *installervsphere.FailureDomain, remoteMachineSets []machineapi.MachineSet, infraID, poolResourcePool string, logger log.FieldLogger) string {
	// Derive expected VMGroup: only host-group zonal FDs have a VMGroup set on their MachineSets.
	vmGroup := ""
	if fd.ZoneType == installervsphere.HostGroupFailureDomain {
		vmGroup = fmt.Sprintf("%s-%s", infraID, fd.Name)
	}

	// Use the MachinePool-level ResourcePool override if set, matching how
	// GenerateMachineSets overrides the FD's ResourcePool before calling the installer.
	expectedRP := fd.Topology.ResourcePool
	if poolResourcePool != "" {
		expectedRP = poolResourcePool
	}

	for _, ms := range remoteMachineSets {
		spec, err := vsphereProviderSpecFromRawExtension(ms.Spec.Template.Spec.ProviderSpec.Value)
		if err != nil {
			logger.WithError(err).WithField("machineSet", ms.Name).Warn("cannot decode VSphereMachineProviderSpec")
			continue
		}
		if spec.Template == "" || spec.Workspace == nil {
			continue
		}

		// 5-field match (mirrors MCO vsphere_helpers.go)
		if spec.Workspace.Datacenter != fd.Topology.Datacenter ||
			spec.Workspace.Datastore != fd.Topology.Datastore ||
			spec.Workspace.Server != fd.Server ||
			spec.Workspace.VMGroup != vmGroup ||
			path.Clean(spec.Workspace.ResourcePool) != path.Clean(expectedRP) {
			continue
		}

		return spec.Template
	}
	return ""
}

// vsphereProviderSpecFromRawExtension unmarshals a JSON-encoded VSphereMachineProviderSpec.
func vsphereProviderSpecFromRawExtension(rawExtension *runtime.RawExtension) (*machineapi.VSphereMachineProviderSpec, error) {
	if rawExtension == nil {
		return &machineapi.VSphereMachineProviderSpec{}, nil
	}
	spec := new(machineapi.VSphereMachineProviderSpec)
	if err := json.Unmarshal(rawExtension.Raw, spec); err != nil {
		return nil, errors.Wrap(err, "error unmarshalling VSphereMachineProviderSpec")
	}
	return spec, nil
}

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
// a list of MachineSets to sync to the remote cluster.
type VSphereActuator struct {
	logger    log.FieldLogger
	templates map[string]string // fd-name → template extracted from remote MachineSets
}

var _ Actuator = &VSphereActuator{}

// NewVSphereActuator is the constructor for building a VSphereActuator.
// Following the GCP actuator pattern, it preprocesses remoteMachineSets into scalar
// data (a per-failure-domain template map) at construction time so that GenerateMachineSets
// can apply it after DeepCopy without mutating the ClusterDeployment.
func NewVSphereActuator(
	remoteMachineSets []machineapi.MachineSet,
	infraID string,
	failureDomains []installervsphere.FailureDomain,
	logger log.FieldLogger,
) (*VSphereActuator, error) {
	templates := buildTemplateMap(failureDomains, remoteMachineSets, infraID, logger)
	return &VSphereActuator{
		logger:    logger,
		templates: templates,
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
		if failureDomain.Topology.Template == "" {
			if tmpl, ok := a.templates[failureDomain.Name]; ok {
				failureDomain.Topology.Template = tmpl
				logger.WithFields(log.Fields{
					"failureDomain": failureDomain.Name,
					"template":      tmpl,
				}).Info("applied backfilled Topology.Template from remote MachineSet")
			}
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

// buildTemplateMap creates a fd-name → template mapping by matching each failure domain
// (where Template is empty) against remote MachineSets using workspace fields.
func buildTemplateMap(failureDomains []installervsphere.FailureDomain, remoteMachineSets []machineapi.MachineSet, infraID string, logger log.FieldLogger) map[string]string {
	templates := make(map[string]string)
	for i := range failureDomains {
		fd := &failureDomains[i]
		if fd.Topology.Template != "" {
			continue
		}
		template := findTemplateForFD(fd, remoteMachineSets, infraID, logger)
		if template != "" {
			templates[fd.Name] = template
			logger.WithFields(log.Fields{
				"failureDomain": fd.Name,
				"template":      template,
			}).Info("found template backfill from remote MachineSet")
		}
	}
	return templates
}

// findTemplateForFD searches remote MachineSets for one whose workspace fields match the
// given failure domain using a 5-field conjunction (MCO PR #5745 pattern):
// Datacenter + Datastore + Server + VMGroup + ResourcePool.
// Always matches against the FD's own ResourcePool since existing remote MachineSets
// were created with the FD default, not a MachinePool-level override.
func findTemplateForFD(fd *installervsphere.FailureDomain, remoteMachineSets []machineapi.MachineSet, infraID string, logger log.FieldLogger) string {
	vmGroup := ""
	if fd.ZoneType == installervsphere.HostGroupFailureDomain {
		vmGroup = fmt.Sprintf("%s-%s", infraID, fd.Name)
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

		if spec.Workspace.Datacenter != fd.Topology.Datacenter ||
			spec.Workspace.Datastore != fd.Topology.Datastore ||
			spec.Workspace.Server != fd.Server ||
			spec.Workspace.VMGroup != vmGroup ||
			path.Clean(spec.Workspace.ResourcePool) != path.Clean(fd.Topology.ResourcePool) {
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

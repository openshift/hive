/*
Copyright 2023 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providerconfig

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-test/deep"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// VSphereProviderConfig holds the provider spec of a VSphere Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type VSphereProviderConfig struct {
	providerConfig machinev1beta1.VSphereMachineProviderSpec
	infrastructure *configv1.Infrastructure
	logger         logr.Logger
}

func (v VSphereProviderConfig) getFailureDomainFromInfrastructure(fd machinev1.VSphereFailureDomain) (*configv1.VSpherePlatformFailureDomainSpec, error) {
	if v.infrastructure.Spec.PlatformSpec.VSphere != nil {
		for _, failureDomain := range v.infrastructure.Spec.PlatformSpec.VSphere.FailureDomains {
			if failureDomain.Name == fd.Name {
				return &failureDomain, nil
			}
		}
	}

	return nil, errFailureDomainNotDefinedInInfrastructure
}

func (v VSphereProviderConfig) getWorkspaceFromFailureDomain(failureDomain *configv1.VSpherePlatformFailureDomainSpec) *machinev1beta1.Workspace {
	topology := failureDomain.Topology
	workspace := &machinev1beta1.Workspace{}

	if len(topology.ComputeCluster) > 0 {
		workspace.ResourcePool = path.Clean(fmt.Sprintf("/%s/Resources", topology.ComputeCluster))
	}

	if len(topology.ResourcePool) > 0 {
		workspace.ResourcePool = topology.ResourcePool
	}

	if len(topology.Datacenter) > 0 {
		workspace.Datacenter = topology.Datacenter
	}

	if len(topology.Datastore) > 0 {
		workspace.Datastore = topology.Datastore
	}

	if len(failureDomain.Server) > 0 {
		workspace.Server = failureDomain.Server
	}

	if len(topology.Folder) > 0 {
		workspace.Folder = topology.Folder
	} else {
		workspace.Folder = fmt.Sprintf("/%s/vm/%s", workspace.Datacenter, v.infrastructure.Status.InfrastructureName)
	}

	return workspace
}

// getTemplateName returns the name of the name of the template.
func getTemplateName(template string) string {
	if strings.Contains(template, "/") {
		return template[strings.LastIndex(template, "/")+1:]
	}

	return template
}

// Diff compares two ProviderConfigs and returns a list of differences,
// or nil if there are none.
func (v VSphereProviderConfig) Diff(other machinev1beta1.VSphereMachineProviderSpec) ([]string, error) {
	// Templates can be provided either with an absolute path or relative.
	// This can result in the control plane nodes rolling out when they dont need to.
	// As long as the OVA name matches that will be considered a match.
	otherTemplate := getTemplateName(other.Template)
	currentTemplate := getTemplateName(v.providerConfig.Template)

	if otherTemplate == currentTemplate {
		other.Template = v.providerConfig.Template
	}

	return deep.Equal(v.providerConfig, other), nil
}

// InjectFailureDomain returns a new VSphereProviderConfig configured with the failure domain.
func (v VSphereProviderConfig) InjectFailureDomain(fd machinev1.VSphereFailureDomain) (VSphereProviderConfig, error) {
	newVSphereProviderConfig := v

	failureDomain, err := newVSphereProviderConfig.getFailureDomainFromInfrastructure(fd)
	if err != nil {
		return newVSphereProviderConfig, fmt.Errorf("unknown failure domain: %w", err)
	}

	newVSphereProviderConfig.providerConfig.Workspace = newVSphereProviderConfig.getWorkspaceFromFailureDomain(failureDomain)
	topology := failureDomain.Topology
	network := newVSphereProviderConfig.providerConfig.Network

	logNetworkInfo(newVSphereProviderConfig.providerConfig.Network, "control plane machine set network before failure domain: %v", v.logger)

	if len(topology.Networks) > 0 {
		networkSpec := machinev1beta1.NetworkSpec{}
		// If original has AddressesFromPools, that means static IP is desired for the CPMS.  Keep that and just add the FD info.
		// Note, CPMS may have no network devices defined relying on FD to provide.
		if len(network.Devices) > 0 && len(network.Devices[0].AddressesFromPools) > 0 {
			networkSpec.Devices = newVSphereProviderConfig.providerConfig.Network.Devices
		}

		// Set the network name for the device from FD.
		for index, network := range topology.Networks {
			if len(networkSpec.Devices) <= index {
				networkSpec.Devices = append(networkSpec.Devices, machinev1beta1.NetworkDeviceSpec{})
			}

			networkSpec.Devices[index].NetworkName = network
		}

		newVSphereProviderConfig.providerConfig.Network = networkSpec
	}

	logNetworkInfo(newVSphereProviderConfig.providerConfig.Network, "control plane machine set network after failure domain: %v", v.logger)

	if len(topology.Template) > 0 {
		newVSphereProviderConfig.providerConfig.Template = topology.Template[strings.LastIndex(topology.Template, "/")+1:]
	} else if len(v.infrastructure.Spec.PlatformSpec.VSphere.FailureDomains) > 0 {
		newVSphereProviderConfig.providerConfig.Template = fmt.Sprintf("%s-rhcos-%s-%s", v.infrastructure.Status.InfrastructureName, failureDomain.Region, failureDomain.Zone)
	}

	return newVSphereProviderConfig, nil
}

// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
func (v VSphereProviderConfig) ExtractFailureDomain() machinev1.VSphereFailureDomain {
	workspace := v.providerConfig.Workspace

	if v.infrastructure.Spec.PlatformSpec.Type != configv1.VSpherePlatformType {
		return machinev1.VSphereFailureDomain{}
	}

	// Older OCP installs will not have PlatformSpec set for infrastructure.
	if v.infrastructure.Spec.PlatformSpec.VSphere == nil {
		return machinev1.VSphereFailureDomain{}
	}

	failureDomains := v.infrastructure.Spec.PlatformSpec.VSphere.FailureDomains

	for _, failureDomain := range failureDomains {
		topology := failureDomain.Topology
		if workspace.Datacenter == topology.Datacenter &&
			workspace.Datastore == topology.Datastore &&
			workspace.Server == failureDomain.Server &&
			path.Clean(workspace.ResourcePool) == path.Clean(topology.ResourcePool) {
			return machinev1.VSphereFailureDomain{
				Name: failureDomain.Name,
			}
		}
	}

	return machinev1.VSphereFailureDomain{}
}

// ResetTopologyRelatedFields resets fields related to topology and VM placement such as the workspace, network, and template.
func (v VSphereProviderConfig) ResetTopologyRelatedFields() ProviderConfig {
	v.providerConfig.Workspace = &machinev1beta1.Workspace{}
	v.providerConfig.Template = ""

	networkSpec := machinev1beta1.NetworkSpec{}
	devices := networkSpec.Devices

	// preserve ippools if they are defined
	for _, network := range v.providerConfig.Network.Devices {
		if len(network.AddressesFromPools) > 0 {
			networkDeviceSpec := machinev1beta1.NetworkDeviceSpec{
				AddressesFromPools: network.AddressesFromPools,
			}
			devices = append(devices, networkDeviceSpec)
		}
	}

	networkSpec.Devices = devices
	v.providerConfig.Network = networkSpec

	return providerConfig{
		platformType: configv1.VSpherePlatformType,
		vsphere: VSphereProviderConfig{
			providerConfig: v.providerConfig,
			infrastructure: v.infrastructure,
		},
	}
}

// Config returns the stored VSphereMachineProviderSpec.
func (v VSphereProviderConfig) Config() machinev1beta1.VSphereMachineProviderSpec {
	return v.providerConfig
}

// newVSphereProviderConfig creates a VSphere type ProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent a VSphereProviderConfig.
func newVSphereProviderConfig(logger logr.Logger, raw *runtime.RawExtension, infrastructure *configv1.Infrastructure) (ProviderConfig, error) {
	var vsphereMachineProviderSpec machinev1beta1.VSphereMachineProviderSpec

	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &vsphereMachineProviderSpec); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	VSphereProviderConfig := VSphereProviderConfig{
		providerConfig: vsphereMachineProviderSpec,
		infrastructure: infrastructure,
		logger:         logger,
	}

	// For networking, we only need to compare the network name.  For static IPs, we can ignore all ip configuration;
	// however, we may need to verify the addressesFromPools is present.
	for index, device := range vsphereMachineProviderSpec.Network.Devices {
		vsphereMachineProviderSpec.Network.Devices[index] = machinev1beta1.NetworkDeviceSpec{}
		if device.NetworkName != "" {
			vsphereMachineProviderSpec.Network.Devices[index].NetworkName = device.NetworkName
		}

		if device.AddressesFromPools != nil {
			vsphereMachineProviderSpec.Network.Devices[index].AddressesFromPools = device.AddressesFromPools
			vsphereMachineProviderSpec.Network.Devices[index].Nameservers = device.Nameservers
		}
	}

	config := providerConfig{
		platformType: configv1.VSpherePlatformType,
		vsphere:      VSphereProviderConfig,
	}

	return config, nil
}

// logNetworkInfo log network info as json for better debugging of data being processed.
func logNetworkInfo(network machinev1beta1.NetworkSpec, msg string, logger logr.Logger) {
	// To limit marshalling to only when log level > 4.
	if logger.GetV() >= 4 {
		jsonOutput, err := json.Marshal(network)
		if err != nil {
			logger.Error(err, "Got error Marshalling NetworkSpec")
		}

		logger.V(4).Info(msg, string(jsonOutput))
	}
}

/*
Copyright 2022 Red Hat, Inc.

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
	"fmt"

	"github.com/go-logr/logr"
	v1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1alpha1 "github.com/openshift/api/machine/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
)

// OpenStackProviderConfig holds the provider spec of an OpenStack Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type OpenStackProviderConfig struct {
	providerConfig machinev1alpha1.OpenstackProviderSpec
}

// InjectFailureDomain returns a new OpenStackProviderConfig configured with the failure domain
// information provided.
func (a OpenStackProviderConfig) InjectFailureDomain(fd machinev1.OpenStackFailureDomain) OpenStackProviderConfig {
	newOpenStackProviderConfig := a

	if fd.AvailabilityZone != "" {
		newOpenStackProviderConfig.providerConfig.AvailabilityZone = fd.AvailabilityZone
	}

	if fd.RootVolume != nil && newOpenStackProviderConfig.providerConfig.RootVolume != nil {
		if fd.RootVolume.AvailabilityZone != "" {
			newOpenStackProviderConfig.providerConfig.RootVolume.Zone = fd.RootVolume.AvailabilityZone
		}

		if fd.RootVolume.VolumeType != "" {
			newOpenStackProviderConfig.providerConfig.RootVolume.VolumeType = fd.RootVolume.VolumeType
		}
	}

	return newOpenStackProviderConfig
}

// ExtractFailureDomain returns an OpenStackFailureDomain based on the failure domain
// information stored within the OpenStackProviderConfig.
func (a OpenStackProviderConfig) ExtractFailureDomain() machinev1.OpenStackFailureDomain {
	var failureDomainRootVolume *machinev1.RootVolume

	if a.providerConfig.RootVolume != nil {
		// Be liberal in accepting an empty rootVolume in the
		// OpenStackFailureDomain. It should count as nil.
		if az, vt := a.providerConfig.RootVolume.Zone, a.providerConfig.RootVolume.VolumeType; az != "" || vt != "" {
			failureDomainRootVolume = &machinev1.RootVolume{
				AvailabilityZone: az,
				VolumeType:       vt,
			}
		}
	}

	return machinev1.OpenStackFailureDomain{
		AvailabilityZone: a.providerConfig.AvailabilityZone,
		RootVolume:       failureDomainRootVolume,
	}
}

// Config returns the stored OpenStackMachineProviderSpec.
func (a OpenStackProviderConfig) Config() machinev1alpha1.OpenstackProviderSpec {
	return a.providerConfig
}

// newOpenStackProviderConfig creates an OpenStack type ProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent
// an OpenStackMachineProviderConfig.
func newOpenStackProviderConfig(logger logr.Logger, raw *runtime.RawExtension) (ProviderConfig, error) {
	openstackProviderSpec := machinev1alpha1.OpenstackProviderSpec{}
	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &openstackProviderSpec); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	openstackProviderConfig := OpenStackProviderConfig{
		providerConfig: openstackProviderSpec,
	}

	config := providerConfig{
		platformType: v1.OpenStackPlatformType,
		openstack:    openstackProviderConfig,
	}

	return config, nil
}

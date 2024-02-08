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
	"encoding/json"
	"fmt"

	v1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// GCPProviderConfig holds the provider spec of a GCP Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type GCPProviderConfig struct {
	providerConfig machinev1beta1.GCPMachineProviderSpec
}

// InjectFailureDomain returns a new GCPProviderConfig configured with the failure domain.
func (g GCPProviderConfig) InjectFailureDomain(fd machinev1.GCPFailureDomain) GCPProviderConfig {
	newGCPProviderConfig := g

	newGCPProviderConfig.providerConfig.Zone = fd.Zone

	return newGCPProviderConfig
}

// ExtractFailureDomain returns a GCPFailureDomain based on the failure domain
// information stored within the GCPProviderConfig.
func (g GCPProviderConfig) ExtractFailureDomain() machinev1.GCPFailureDomain {
	return machinev1.GCPFailureDomain{
		Zone: g.providerConfig.Zone,
	}
}

// Config returns the stored GCPMachineProviderSpec.
func (g GCPProviderConfig) Config() machinev1beta1.GCPMachineProviderSpec {
	return g.providerConfig
}

// newGCPProviderConfig creates a GCP type ProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent a GCPProviderConfig.
func newGCPProviderConfig(raw *runtime.RawExtension) (ProviderConfig, error) {
	var gcpMachineProviderSpec machinev1beta1.GCPMachineProviderSpec
	if err := json.Unmarshal(raw.Raw, &gcpMachineProviderSpec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GCP provider config: %w", err)
	}

	gcpProviderConfig := GCPProviderConfig{
		providerConfig: gcpMachineProviderSpec,
	}

	config := providerConfig{
		platformType: v1.GCPPlatformType,
		gcp:          gcpProviderConfig,
	}

	return config, nil
}

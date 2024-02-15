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
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AzureProviderConfig holds the provider spec of an Azure Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type AzureProviderConfig struct {
	providerConfig machinev1beta1.AzureMachineProviderSpec
}

// InjectFailureDomain returns a new AzureProviderConfig configured with the failure domain
// information provided.
func (a AzureProviderConfig) InjectFailureDomain(fd machinev1.AzureFailureDomain) AzureProviderConfig {
	newAzureProviderConfig := a

	if fd.Zone != "" {
		newAzureProviderConfig.providerConfig.Zone = fd.Zone
	}

	if fd.Subnet != "" {
		newAzureProviderConfig.providerConfig.Subnet = fd.Subnet
	}

	return newAzureProviderConfig
}

// ExtractFailureDomain returns an AzureFailureDomain based on the failure domain
// information stored within the AzureProviderConfig.
func (a AzureProviderConfig) ExtractFailureDomain() machinev1.AzureFailureDomain {
	return machinev1.AzureFailureDomain{
		Zone:   a.providerConfig.Zone,
		Subnet: a.providerConfig.Subnet,
	}
}

// Config returns the stored AzureMachineProviderSpec.
func (a AzureProviderConfig) Config() machinev1beta1.AzureMachineProviderSpec {
	return a.providerConfig
}

// newAzureProviderConfig creates an Azure type ProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent
// an AzureMachineProviderConfig.
func newAzureProviderConfig(logger logr.Logger, raw *runtime.RawExtension) (ProviderConfig, error) {
	azureMachineProviderSpec := machinev1beta1.AzureMachineProviderSpec{}

	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &azureMachineProviderSpec); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	azureProviderConfig := AzureProviderConfig{
		providerConfig: azureMachineProviderSpec,
	}

	config := providerConfig{
		platformType: v1.AzurePlatformType,
		azure:        azureProviderConfig,
	}

	return config, nil
}

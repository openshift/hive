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
	"fmt"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// NutanixProviderConfig is a wrapper around machinev1.NutanixMachineProviderConfig.
type NutanixProviderConfig struct {
	providerConfig machinev1.NutanixMachineProviderConfig
}

// Config returns the stored NutanixMachineProviderConfig.
func (n NutanixProviderConfig) Config() machinev1.NutanixMachineProviderConfig {
	return n.providerConfig
}

func newNutanixProviderConfig(logger logr.Logger, raw *runtime.RawExtension) (ProviderConfig, error) {
	nutanixMachineProviderconfig := machinev1.NutanixMachineProviderConfig{}

	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &nutanixMachineProviderconfig); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	npc := NutanixProviderConfig{
		providerConfig: nutanixMachineProviderconfig,
	}

	config := providerConfig{
		platformType: configv1.NutanixPlatformType,
		nutanix:      npc,
	}

	return config, nil
}

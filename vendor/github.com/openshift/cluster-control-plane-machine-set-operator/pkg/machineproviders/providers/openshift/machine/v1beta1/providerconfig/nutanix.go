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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var errInvalidFailureDomainName = errors.New("invalid failure domain name")

// NutanixProviderConfig is a wrapper around machinev1.NutanixMachineProviderConfig.
type NutanixProviderConfig struct {
	providerConfig machinev1.NutanixMachineProviderConfig
	infrastructure *configv1.Infrastructure
}

// Config returns the stored NutanixMachineProviderConfig.
func (n NutanixProviderConfig) Config() machinev1.NutanixMachineProviderConfig {
	return n.providerConfig
}

// Infrastructure returns the stored *configv1.Infrastructure.
func (n NutanixProviderConfig) Infrastructure() *configv1.Infrastructure {
	return n.infrastructure
}

func newNutanixProviderConfig(logger logr.Logger, raw *runtime.RawExtension, infrastructure *configv1.Infrastructure) (ProviderConfig, error) {
	nutanixMachineProviderconfig := machinev1.NutanixMachineProviderConfig{}

	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &nutanixMachineProviderconfig); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	npc := NutanixProviderConfig{
		providerConfig: nutanixMachineProviderconfig,
		infrastructure: infrastructure,
	}

	config := providerConfig{
		platformType: configv1.NutanixPlatformType,
		nutanix:      npc,
	}

	return config, nil
}

// GetFailureDomainByName returns the NutanixFailureDomain if the input name referenced
// failureDomain is configured in the Infrastructure resource.
func (n NutanixProviderConfig) GetFailureDomainByName(failureDomainName string) (*configv1.NutanixFailureDomain, error) {
	if failureDomainName == "" {
		return nil, fmt.Errorf("empty failure domain name. %w", errInvalidFailureDomainName)
	}

	for _, fd := range n.infrastructure.Spec.PlatformSpec.Nutanix.FailureDomains {
		if fd.Name == failureDomainName {
			return &fd, nil
		}
	}

	return nil, fmt.Errorf("the failure domain with name %q is not defined in the infrastructure resource. %w", failureDomainName, errInvalidFailureDomainName)
}

// InjectFailureDomain returns a new NutanixProviderConfig configured with the failure domain information provided.
func (n NutanixProviderConfig) InjectFailureDomain(fdRef machinev1.NutanixFailureDomainReference) (NutanixProviderConfig, error) {
	newConfig := n

	if fdRef.Name == "" {
		return newConfig, nil
	}

	fd, err := newConfig.GetFailureDomainByName(fdRef.Name)
	if err != nil {
		return newConfig, fmt.Errorf("unknown failure domain: %w", err)
	}

	// update the providerConfig fields that is defined in the referenced failure domain
	newConfig.providerConfig.FailureDomain = &machinev1.NutanixFailureDomainReference{
		Name: fd.Name,
	}
	// update Cluster
	newConfig.providerConfig.Cluster = machinev1.NutanixResourceIdentifier{
		Name: fd.Cluster.Name,
		UUID: fd.Cluster.UUID,
	}
	if fd.Cluster.Type == configv1.NutanixIdentifierName {
		newConfig.providerConfig.Cluster.Type = machinev1.NutanixIdentifierName
	} else if fd.Cluster.Type == configv1.NutanixIdentifierUUID {
		newConfig.providerConfig.Cluster.Type = machinev1.NutanixIdentifierUUID
	}

	// update Subnets
	newConfig.providerConfig.Subnets = []machinev1.NutanixResourceIdentifier{}

	for _, fdSubnet := range fd.Subnets {
		pcSubnet := machinev1.NutanixResourceIdentifier{
			Name: fdSubnet.Name,
			UUID: fdSubnet.UUID,
		}
		if fdSubnet.Type == configv1.NutanixIdentifierName {
			pcSubnet.Type = machinev1.NutanixIdentifierName
		} else if fdSubnet.Type == configv1.NutanixIdentifierUUID {
			pcSubnet.Type = machinev1.NutanixIdentifierUUID
		}

		newConfig.providerConfig.Subnets = append(newConfig.providerConfig.Subnets, pcSubnet)
	}

	return newConfig, nil
}

// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
func (n NutanixProviderConfig) ExtractFailureDomain() machinev1.NutanixFailureDomainReference {
	if n.providerConfig.FailureDomain == nil {
		return machinev1.NutanixFailureDomainReference{}
	}

	if fd, _ := n.GetFailureDomainByName(n.providerConfig.FailureDomain.Name); fd != nil {
		return *n.providerConfig.FailureDomain
	}

	return machinev1.NutanixFailureDomainReference{}
}

// ResetFailureDomainRelatedFields resets fields related to failure domain.
func (n NutanixProviderConfig) ResetFailureDomainRelatedFields() ProviderConfig {
	n.providerConfig.Cluster = machinev1.NutanixResourceIdentifier{}
	n.providerConfig.Subnets = []machinev1.NutanixResourceIdentifier{}
	n.providerConfig.FailureDomain = nil

	return providerConfig{
		platformType: configv1.NutanixPlatformType,
		nutanix: NutanixProviderConfig{
			providerConfig: n.providerConfig,
			infrastructure: n.infrastructure,
		},
	}
}

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
	"errors"
	"fmt"
	"reflect"

	"github.com/go-test/deep"
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/failuredomain"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// errMismatchedPlatformTypes is an error used when two provider configs
	// are being compared but are from different platform types.
	errMismatchedPlatformTypes = errors.New("mistmatched platform types")

	// errUnsupportedPlatformType is an error used when an unknown platform
	// type is configured within the failure domain config.
	errUnsupportedPlatformType = errors.New("unsupported platform type")

	// errNilProviderSpec is an error used when provider spec is nil.
	errNilProviderSpec = errors.New("provider spec is nil")

	// errNilFailureDomain is an error used when when nil value is present and failure domain is expected.
	errNilFailureDomain = errors.New("failure domain is nil")
)

// ProviderConfig is an interface that allows external code to interact
// with provider configuration across different platform types.
type ProviderConfig interface {
	// InjectFailureDomain is used to inject a failure domain into the ProviderConfig.
	// The returned ProviderConfig will be a copy of the current ProviderConfig with
	// the new failure domain injected.
	InjectFailureDomain(failuredomain.FailureDomain) (ProviderConfig, error)

	// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
	ExtractFailureDomain() failuredomain.FailureDomain

	// Equal compares two ProviderConfigs to determine whether or not they are equal.
	Equal(ProviderConfig) (bool, error)

	// Diff compares two ProviderConfigs and returns a list of differences,
	// or nil if there are none.
	Diff(ProviderConfig) ([]string, error)

	// RawConfig marshalls the configuration into a JSON byte slice.
	RawConfig() ([]byte, error)

	// Type returns the platform type of the provider config.
	Type() configv1.PlatformType

	// AWS returns the AWSProviderConfig if the platform type is AWS.
	AWS() AWSProviderConfig

	// Azure returns the AzureProviderConfig if the platform type is Azure.
	Azure() AzureProviderConfig

	// GCP returns the GCPProviderConfig if the platform type is GCP.
	GCP() GCPProviderConfig

	// Generic returns the GenericProviderConfig if we are on a platform that is using generic provider abstraction.
	Generic() GenericProviderConfig
}

// NewProviderConfigFromMachineTemplate creates a new ProviderConfig from the provided machine template.
func NewProviderConfigFromMachineTemplate(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (ProviderConfig, error) {
	platformType, err := getPlatformTypeFromMachineTemplate(tmpl)
	if err != nil {
		return nil, fmt.Errorf("could not determine platform type: %w", err)
	}

	return newProviderConfigFromProviderSpec(tmpl.Spec.ProviderSpec, platformType)
}

// NewProviderConfigFromMachineSpec creates a new ProviderConfig from the provided machineSpec object.
func NewProviderConfigFromMachineSpec(machineSpec machinev1beta1.MachineSpec) (ProviderConfig, error) {
	platformType, err := getPlatformTypeFromProviderSpec(machineSpec.ProviderSpec)
	if err != nil {
		return nil, fmt.Errorf("could not determine platform type: %w", err)
	}

	return newProviderConfigFromProviderSpec(machineSpec.ProviderSpec, platformType)
}

func newProviderConfigFromProviderSpec(providerSpec machinev1beta1.ProviderSpec, platformType configv1.PlatformType) (ProviderConfig, error) {
	if providerSpec.Value == nil {
		return nil, errNilProviderSpec
	}

	switch platformType {
	case configv1.AWSPlatformType:
		return newAWSProviderConfig(providerSpec.Value)
	case configv1.AzurePlatformType:
		return newAzureProviderConfig(providerSpec.Value)
	case configv1.GCPPlatformType:
		return newGCPProviderConfig(providerSpec.Value)
	case configv1.NonePlatformType:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, platformType)
	default:
		return newGenericProviderConfig(providerSpec.Value, platformType)
	}
}

// providerConfig is an implementation of the ProviderConfig interface.
type providerConfig struct {
	platformType configv1.PlatformType
	aws          AWSProviderConfig
	azure        AzureProviderConfig
	gcp          GCPProviderConfig
	generic      GenericProviderConfig
}

// InjectFailureDomain is used to inject a failure domain into the ProviderConfig.
// The returned ProviderConfig will be a copy of the current ProviderConfig with
// the new failure domain injected.
func (p providerConfig) InjectFailureDomain(fd failuredomain.FailureDomain) (ProviderConfig, error) {
	if fd == nil {
		return nil, errNilFailureDomain
	}

	newConfig := p

	switch p.platformType {
	case configv1.AWSPlatformType:
		newConfig.aws = p.AWS().InjectFailureDomain(fd.AWS())
	case configv1.AzurePlatformType:
		newConfig.azure = p.Azure().InjectFailureDomain(fd.Azure())
	case configv1.GCPPlatformType:
		newConfig.gcp = p.GCP().InjectFailureDomain(fd.GCP())
	case configv1.NonePlatformType:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, p.platformType)
	}

	return newConfig, nil
}

// ExtractFailureDomain is used to extract a failure domain from the ProviderConfig.
func (p providerConfig) ExtractFailureDomain() failuredomain.FailureDomain {
	switch p.platformType {
	case configv1.AWSPlatformType:
		return failuredomain.NewAWSFailureDomain(p.AWS().ExtractFailureDomain())
	case configv1.AzurePlatformType:
		return failuredomain.NewAzureFailureDomain(p.Azure().ExtractFailureDomain())
	case configv1.GCPPlatformType:
		return failuredomain.NewGCPFailureDomain(p.GCP().ExtractFailureDomain())
	case configv1.NonePlatformType:
		return nil
	default:
		return p.Generic().ExtractFailureDomain()
	}
}

// Diff compares two ProviderConfigs and returns a list of differences,
// or nil if there are none.
func (p providerConfig) Diff(other ProviderConfig) ([]string, error) {
	if other == nil {
		return nil, nil
	}

	if p.platformType != other.Type() {
		return nil, errMismatchedPlatformTypes
	}

	switch p.platformType {
	case configv1.AWSPlatformType:
		return deep.Equal(p.aws.providerConfig, other.AWS().providerConfig), nil
	case configv1.AzurePlatformType:
		return deep.Equal(p.azure.providerConfig, other.Azure().providerConfig), nil
	case configv1.GCPPlatformType:
		return deep.Equal(p.gcp.providerConfig, other.GCP().providerConfig), nil
	case configv1.NonePlatformType:
		return nil, errUnsupportedPlatformType
	default:
		return deep.Equal(p.generic.providerSpec, other.Generic().providerSpec), nil
	}
}

// Equal compares two ProviderConfigs to determine whether or not they are equal.
func (p providerConfig) Equal(other ProviderConfig) (bool, error) {
	if other == nil {
		return false, nil
	}

	if p.platformType != other.Type() {
		return false, errMismatchedPlatformTypes
	}

	switch p.platformType {
	case configv1.AWSPlatformType:
		return reflect.DeepEqual(p.aws.providerConfig, other.AWS().providerConfig), nil
	case configv1.AzurePlatformType:
		return reflect.DeepEqual(p.azure.providerConfig, other.Azure().providerConfig), nil
	case configv1.GCPPlatformType:
		return reflect.DeepEqual(p.gcp.providerConfig, other.GCP().providerConfig), nil
	case configv1.NonePlatformType:
		return false, errUnsupportedPlatformType
	default:
		return reflect.DeepEqual(p.generic.providerSpec, other.Generic().providerSpec), nil
	}
}

// RawConfig marshalls the configuration into a JSON byte slice.
func (p providerConfig) RawConfig() ([]byte, error) {
	var (
		rawConfig []byte
		err       error
	)

	switch p.platformType {
	case configv1.AWSPlatformType:
		rawConfig, err = json.Marshal(p.aws.providerConfig)
	case configv1.AzurePlatformType:
		rawConfig, err = json.Marshal(p.azure.providerConfig)
	case configv1.GCPPlatformType:
		rawConfig, err = json.Marshal(p.gcp.providerConfig)
	case configv1.NonePlatformType:
		return nil, errUnsupportedPlatformType
	default:
		rawConfig, err = p.generic.providerSpec.Raw, nil
	}

	if err != nil {
		return nil, fmt.Errorf("could not marshal provider config: %w", err)
	}

	return rawConfig, nil
}

// Type returns the platform type of the provider config.
func (p providerConfig) Type() configv1.PlatformType {
	return p.platformType
}

// AWS returns the AWSProviderConfig if the platform type is AWS.
func (p providerConfig) AWS() AWSProviderConfig {
	return p.aws
}

// Azure returns the AzureProviderConfig if the platform type is Azure.
func (p providerConfig) Azure() AzureProviderConfig {
	return p.azure
}

// GCP returns the GCPProviderConfig if the platform type is GCP.
func (p providerConfig) GCP() GCPProviderConfig {
	return p.gcp
}

// Generic returns the GenericProviderConfig if the platform type is generic.
func (p providerConfig) Generic() GenericProviderConfig {
	return p.generic
}

// getPlatformTypeFromProviderSpecKind determines machine platform from providerSpec kind.
// When platform is unknown, it returns "UnknownPlatform".
func getPlatformTypeFromProviderSpecKind(kind string) configv1.PlatformType {
	var providerSpecKindToPlatformType = map[string]configv1.PlatformType{
		"AWSMachineProviderConfig": configv1.AWSPlatformType,
		"AzureMachineProviderSpec": configv1.AzurePlatformType,
		"GCPMachineProviderSpec":   configv1.GCPPlatformType,
	}

	platformType, ok := providerSpecKindToPlatformType[kind]

	// Attempt to operate on unknown platforms. This should work if the platform does not require failure domains support.
	if !ok {
		return "UnknownPlatform"
	}

	return platformType
}

// getPlatformTypeFromMachineTemplate extracts the platform type from the Machine template.
// This can either be gathered from the platform type within the template failure domains,
// or if that isn't present, by inspecting the providerSpec kind and inferring from there
// what the configured platform type is.
func getPlatformTypeFromMachineTemplate(tmpl machinev1.OpenShiftMachineV1Beta1MachineTemplate) (configv1.PlatformType, error) {
	platformType := tmpl.FailureDomains.Platform
	if platformType != "" {
		return platformType, nil
	}

	return getPlatformTypeFromProviderSpec(tmpl.Spec.ProviderSpec)
}

// getPlatformTypeFromProviderSpec determines machine platform from the providerSpec.
// The providerSpec object's kind field is unmarshalled and the platform type is inferred from it.
func getPlatformTypeFromProviderSpec(providerSpec machinev1beta1.ProviderSpec) (configv1.PlatformType, error) {
	// Simple type for unmarshalling providerSpec kind.
	type providerSpecKind struct {
		metav1.TypeMeta `json:",inline"`
	}

	providerKind := providerSpecKind{}

	if providerSpec.Value == nil {
		return "", errNilProviderSpec
	}

	if err := json.Unmarshal(providerSpec.Value.Raw, &providerKind); err != nil {
		return "", fmt.Errorf("could not unmarshal provider spec: %w", err)
	}

	return getPlatformTypeFromProviderSpecKind(providerKind.Kind), nil
}

// ExtractFailureDomainsFromMachines creates list of FailureDomains extracted from the provided list of machines.
func ExtractFailureDomainsFromMachines(machines []machinev1beta1.Machine) ([]failuredomain.FailureDomain, error) {
	machineFailureDomains := failuredomain.NewSet()

	for _, machine := range machines {
		providerconfig, err := NewProviderConfigFromMachineSpec(machine.Spec)
		if err != nil {
			return nil, fmt.Errorf("error getting failure domain from machine %s: %w", machine.Name, err)
		}

		machineFailureDomains.Insert(providerconfig.ExtractFailureDomain())
	}

	return machineFailureDomains.List(), nil
}

// ExtractFailureDomainFromMachine FailureDomain extracted from the provided machine.
func ExtractFailureDomainFromMachine(machine machinev1beta1.Machine) (failuredomain.FailureDomain, error) {
	providerConfig, err := NewProviderConfigFromMachineSpec(machine.Spec)
	if err != nil {
		return nil, fmt.Errorf("error getting failure domain from machine %s: %w", machine.Name, err)
	}

	return providerConfig.ExtractFailureDomain(), nil
}

// ExtractFailureDomainsFromMachineSets creates list of FailureDomains extracted from the provided list of machineSets.
func ExtractFailureDomainsFromMachineSets(machineSets []machinev1beta1.MachineSet) ([]failuredomain.FailureDomain, error) {
	machineSetFailureDomains := failuredomain.NewSet()

	for _, machineSet := range machineSets {
		providerconfig, err := NewProviderConfigFromMachineSpec(machineSet.Spec.Template.Spec)
		if err != nil {
			return nil, fmt.Errorf("error getting failure domain from machineSet %s: %w", machineSet.Name, err)
		}

		machineSetFailureDomains.Insert(providerconfig.ExtractFailureDomain())
	}

	return machineSetFailureDomains.List(), nil
}

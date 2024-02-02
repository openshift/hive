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
	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AWSProviderConfig holds the provider spec of an AWS Machine.
// It allows external code to extract and inject failure domain information,
// as well as gathering the stored config.
type AWSProviderConfig struct {
	providerConfig machinev1beta1.AWSMachineProviderConfig
}

// InjectFailureDomain returns a new AWSProviderConfig configured with the failure domain
// information provided.
func (a AWSProviderConfig) InjectFailureDomain(fd machinev1.AWSFailureDomain) AWSProviderConfig {
	newAWSProviderConfig := a

	if fd.Placement.AvailabilityZone != "" {
		newAWSProviderConfig.providerConfig.Placement.AvailabilityZone = fd.Placement.AvailabilityZone
	}

	if fd.Subnet != nil {
		newAWSProviderConfig.providerConfig.Subnet = convertAWSResourceReferenceV1ToV1Beta1(fd.Subnet)
	}

	return newAWSProviderConfig
}

// ExtractFailureDomain returns an AWSFailureDomain based on the failure domain
// information stored within the AWSProviderConfig.
func (a AWSProviderConfig) ExtractFailureDomain() machinev1.AWSFailureDomain {
	return machinev1.AWSFailureDomain{
		Placement: machinev1.AWSFailureDomainPlacement{
			AvailabilityZone: a.providerConfig.Placement.AvailabilityZone,
		},
		Subnet: convertAWSResourceReferenceV1Beta1ToV1(a.providerConfig.Subnet),
	}
}

// Config returns the stored AWSMachineProviderConfig.
func (a AWSProviderConfig) Config() machinev1beta1.AWSMachineProviderConfig {
	return a.providerConfig
}

// newAWSProviderConfig creates an AWS type ProviderConfig from the raw extension.
// It should return an error if the provided RawExtension does not represent
// an AWSMachineProviderConfig.
func newAWSProviderConfig(logger logr.Logger, raw *runtime.RawExtension) (ProviderConfig, error) {
	if raw == nil {
		return nil, errNilProviderSpec
	}

	awsMachineProviderConfig := machinev1beta1.AWSMachineProviderConfig{}

	if err := checkForUnknownFieldsInProviderSpecAndUnmarshal(logger, raw, &awsMachineProviderConfig); err != nil {
		return nil, fmt.Errorf("failed to check for unknown fields in the provider spec: %w", err)
	}

	awsProviderConfig := AWSProviderConfig{
		providerConfig: awsMachineProviderConfig,
	}

	config := providerConfig{
		platformType: configv1.AWSPlatformType,
		aws:          awsProviderConfig,
	}

	return config, nil
}

// ConvertAWSResourceReferenceV1Beta1ToV1 creates a machinev1.awsResourceReference from machinev1beta1.awsResourceReference.
func convertAWSResourceReferenceV1Beta1ToV1(referenceV1Beta1 machinev1beta1.AWSResourceReference) *machinev1.AWSResourceReference {
	referenceV1 := &machinev1.AWSResourceReference{}

	if referenceV1Beta1.ID != nil {
		referenceV1.Type = machinev1.AWSIDReferenceType
		referenceV1.ID = referenceV1Beta1.ID

		return referenceV1
	}

	if referenceV1Beta1.Filters != nil {
		referenceV1.Type = machinev1.AWSFiltersReferenceType

		referenceV1.Filters = &[]machinev1.AWSResourceFilter{}
		for _, filter := range referenceV1Beta1.Filters {
			*referenceV1.Filters = append(*referenceV1.Filters, machinev1.AWSResourceFilter{
				Name:   filter.Name,
				Values: filter.Values,
			})
		}

		return referenceV1
	}

	if referenceV1Beta1.ARN != nil {
		referenceV1.Type = machinev1.AWSARNReferenceType
		referenceV1.ARN = referenceV1Beta1.ARN

		return referenceV1
	}

	return nil
}

// ConvertAWSResourceReferenceV1ToV1Beta1 creates a machinev1beta1.awsResourceReference from machinev1.awsResourceReference.
func convertAWSResourceReferenceV1ToV1Beta1(referenceV1 *machinev1.AWSResourceReference) machinev1beta1.AWSResourceReference {
	referenceV1Beta1 := machinev1beta1.AWSResourceReference{}

	if referenceV1 == nil {
		return machinev1beta1.AWSResourceReference{}
	}

	switch referenceV1.Type {
	case machinev1.AWSIDReferenceType:
		referenceV1Beta1.ID = referenceV1.ID

		return referenceV1Beta1
	case machinev1.AWSFiltersReferenceType:
		referenceV1Beta1.Filters = []machinev1beta1.Filter{}
		for _, filter := range *referenceV1.Filters {
			referenceV1Beta1.Filters = append(referenceV1Beta1.Filters, machinev1beta1.Filter{
				Name:   filter.Name,
				Values: filter.Values,
			})
		}

		return referenceV1Beta1
	case machinev1.AWSARNReferenceType:
		referenceV1Beta1.ARN = referenceV1.ARN

		return referenceV1Beta1
	}

	return referenceV1Beta1
}

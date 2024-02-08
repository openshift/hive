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

package failuredomain

import (
	"errors"
	"fmt"
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	machinev1 "github.com/openshift/api/machine/v1"
)

const (
	// unknownFailureDomain is used as the string representation of a failure
	// domain when the platform type is unrecognised.
	unknownFailureDomain = "<unknown>"
)

var (
	// errUnsupportedPlatformType is an error used when an unknown platform
	// type is configured within the failure domain config.
	errUnsupportedPlatformType = errors.New("unsupported platform type")

	// errMissingFailureDomain is an error used when failure domain platform is set
	// but the failure domain list is nil.
	errMissingFailureDomain = errors.New("missing failure domain configuration")
)

// FailureDomain is an interface that allows external code to interact with
// failure domains across different platform types.
type FailureDomain interface {
	// String returns a string representation of the failure domain.
	String() string

	// Type returns the platform type of the failure domain.
	Type() configv1.PlatformType

	// AWS returns the AWSFailureDomain if the platform type is AWS.
	AWS() machinev1.AWSFailureDomain

	// AWS returns the AzureFailureDomain if the platform type is Azure.
	Azure() machinev1.AzureFailureDomain

	// GCP returns the GCPFailureDomain if the platform type is GCP.
	GCP() machinev1.GCPFailureDomain

	// Equal compares the underlying failure domain.
	Equal(other FailureDomain) bool
}

// failureDomain holds an implementation of the FailureDomain interface.
type failureDomain struct {
	platformType configv1.PlatformType

	aws   machinev1.AWSFailureDomain
	azure machinev1.AzureFailureDomain
	gcp   machinev1.GCPFailureDomain
}

// String returns a string representation of the failure domain.
func (f failureDomain) String() string {
	switch f.platformType {
	case configv1.AWSPlatformType:
		return awsFailureDomainToString(f.aws)
	case configv1.AzurePlatformType:
		return azureFailureDomainToString(f.azure)
	case configv1.GCPPlatformType:
		return gcpFailureDomainToString(f.gcp)
	default:
		return fmt.Sprintf("%sFailureDomain{}", f.platformType)
	}
}

// Type returns the platform type of the failure domain.
func (f failureDomain) Type() configv1.PlatformType {
	return f.platformType
}

// AWS returns the AWSFailureDomain if the platform type is AWS.
func (f failureDomain) AWS() machinev1.AWSFailureDomain {
	return f.aws
}

// Azure returns the AzureFailureDomain if the platform type is Azure.
func (f failureDomain) Azure() machinev1.AzureFailureDomain {
	return f.azure
}

// GCP returns the GCPFailureDomain if the platform type is GCP.
func (f failureDomain) GCP() machinev1.GCPFailureDomain {
	return f.gcp
}

// Equal compares the underlying failure domain.
func (f failureDomain) Equal(other FailureDomain) bool {
	if other == nil {
		return false
	}

	if f.platformType != other.Type() {
		return false
	}

	switch f.platformType {
	case configv1.AWSPlatformType:
		return reflect.DeepEqual(f.AWS(), other.AWS())
	case configv1.AzurePlatformType:
		return f.azure == other.Azure()
	case configv1.GCPPlatformType:
		return f.gcp == other.GCP()
	}

	return true
}

// NewFailureDomains creates a set of FailureDomains representing the input failure
// domains held within the ControlPlaneMachineSet.
func NewFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	switch failureDomains.Platform {
	case configv1.AWSPlatformType:
		return newAWSFailureDomains(failureDomains)
	case configv1.AzurePlatformType:
		return newAzureFailureDomains(failureDomains)
	case configv1.GCPPlatformType:
		return newGCPFailureDomains(failureDomains)
	case configv1.PlatformType(""):
		// An empty failure domains definition is allowed.
		return nil, nil
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedPlatformType, failureDomains.Platform)
	}
}

// newAWSFailureDomains constructs a slice of AWS FailureDomain from machinev1.FailureDomains.
func newAWSFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}
	if failureDomains.AWS == nil {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, failureDomain := range *failureDomains.AWS {
		foundFailureDomains = append(foundFailureDomains, NewAWSFailureDomain(failureDomain))
	}

	return foundFailureDomains, nil
}

// newAzureFailureDomains constructs a slice of Azure FailureDomain from machinev1.FailureDomains.
func newAzureFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}
	if failureDomains.Azure == nil {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, failureDomain := range *failureDomains.Azure {
		foundFailureDomains = append(foundFailureDomains, NewAzureFailureDomain(failureDomain))
	}

	return foundFailureDomains, nil
}

// newGCPFailureDomains constructs a slice of GCP FailureDomain from machinev1.FailureDomains.
func newGCPFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}
	if failureDomains.GCP == nil {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, failureDomain := range *failureDomains.GCP {
		foundFailureDomains = append(foundFailureDomains, NewGCPFailureDomain(failureDomain))
	}

	return foundFailureDomains, nil
}

// NewAWSFailureDomain creates an AWS failure domain from the machinev1.AWSFailureDomain.
// Note this is exported to allow other packages to construct individual failure domains
// in tests.
func NewAWSFailureDomain(fd machinev1.AWSFailureDomain) FailureDomain {
	return &failureDomain{
		platformType: configv1.AWSPlatformType,
		aws:          fd,
	}
}

// NewAzureFailureDomain creates an Azure failure domain from the machinev1.AzureFailureDomain.
func NewAzureFailureDomain(fd machinev1.AzureFailureDomain) FailureDomain {
	return &failureDomain{
		platformType: configv1.AzurePlatformType,
		azure:        fd,
	}
}

// NewGCPFailureDomain creates a GCP failure domain from the machinev1.GCPFailureDomain.
func NewGCPFailureDomain(fd machinev1.GCPFailureDomain) FailureDomain {
	return &failureDomain{
		platformType: configv1.GCPPlatformType,
		gcp:          fd,
	}
}

// NewGenericFailureDomain creates a dummy failure domain for generic platforms that don't support failure domains.
func NewGenericFailureDomain() FailureDomain {
	return failureDomain{}
}

// azString formats AvailabilityZone for awsFailureDomainToString function.
func azString(az string) string {
	if az == "" {
		return ""
	}

	return fmt.Sprintf("AvailabilityZone:%s, ", az)
}

// awsFailureDomainToString converts the AWSFailureDomain into a string.
// The types are slightly changed to be more human readable and nil values are omitted.
func awsFailureDomainToString(fd machinev1.AWSFailureDomain) string {
	// Availability zone only
	if fd.Placement.AvailabilityZone != "" && fd.Subnet == nil {
		return fmt.Sprintf("AWSFailureDomain{AvailabilityZone:%s}", fd.Placement.AvailabilityZone)
	}

	// Only subnet or both
	if fd.Subnet != nil {
		switch fd.Subnet.Type {
		case machinev1.AWSARNReferenceType:
			if fd.Subnet.ARN != nil {
				return fmt.Sprintf("AWSFailureDomain{%sSubnet:{Type:%s, Value:%s}}", azString(fd.Placement.AvailabilityZone), fd.Subnet.Type, *fd.Subnet.ARN)
			}
		case machinev1.AWSFiltersReferenceType:
			if fd.Subnet.Filters != nil {
				return fmt.Sprintf("AWSFailureDomain{%sSubnet:{Type:%s, Value:%+v}}", azString(fd.Placement.AvailabilityZone), fd.Subnet.Type, fd.Subnet.Filters)
			}
		case machinev1.AWSIDReferenceType:
			if fd.Subnet.ID != nil {
				return fmt.Sprintf("AWSFailureDomain{%sSubnet:{Type:%s, Value:%s}}", azString(fd.Placement.AvailabilityZone), fd.Subnet.Type, *fd.Subnet.ID)
			}
		}
	}

	// If the previous attempts to find a suitable string do not work,
	// this should catch the fallthrough.
	return unknownFailureDomain
}

// azureFailureDomainToString converts the AzureFailureDomain into a string.
func azureFailureDomainToString(fd machinev1.AzureFailureDomain) string {
	if fd.Zone != "" {
		return fmt.Sprintf("AzureFailureDomain{Zone:%s}", fd.Zone)
	}

	return unknownFailureDomain
}

// gcpFailureDomainToString converts the GCPFailureDomain into a string.
func gcpFailureDomainToString(fd machinev1.GCPFailureDomain) string {
	if fd.Zone != "" {
		return fmt.Sprintf("GCPFailureDomain{Zone:%s}", fd.Zone)
	}

	return unknownFailureDomain
}

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
	"strings"

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

	// errMissingTemplateFailureDomain is an error used when you attempt to combine a failure domain with a nil template failure domain.
	errMissingTemplateFailureDomain = errors.New("failure domain extracted from machine template is nil")

	// errMismatchedPlatformType is an error used when attempting to compare failure domains of different platform types.
	errMismatchedPlatformType = errors.New("platform types do not match")
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

	// OpenStack returns the OpenStackFailureDomain if the platform type is OpenStack.
	OpenStack() machinev1.OpenStackFailureDomain

	// VSphere returns the VSphereFailureDomain if the platform type is VSphere.
	VSphere() machinev1.VSphereFailureDomain

	// Nutanix returns the NutanixFailureDomainReference if the platform type is Nutanix.
	Nutanix() machinev1.NutanixFailureDomainReference

	// Equal compares the underlying failure domain.
	Equal(other FailureDomain) bool

	// Complete ensures that any empty value in the failure domain is populated based on the template failure domain, to create a fully specified failure domain.
	Complete(templateFailureDomain FailureDomain) (FailureDomain, error)
}

// failureDomain holds an implementation of the FailureDomain interface.
type failureDomain struct {
	platformType configv1.PlatformType

	aws       machinev1.AWSFailureDomain
	azure     machinev1.AzureFailureDomain
	gcp       machinev1.GCPFailureDomain
	openstack machinev1.OpenStackFailureDomain
	vsphere   machinev1.VSphereFailureDomain
	nutanix   machinev1.NutanixFailureDomainReference
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
	case configv1.OpenStackPlatformType:
		return openstackFailureDomainToString(f.openstack)
	case configv1.VSpherePlatformType:
		return vsphereFailureDomainToString(f.vsphere)
	case configv1.NutanixPlatformType:
		return nutanixFailureDomainToString(f.nutanix)
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

// OpenStack returns the OpenStackFailureDomain if the platform type is OpenStack.
func (f failureDomain) OpenStack() machinev1.OpenStackFailureDomain {
	return f.openstack
}

// VSphere returns the VSphereFailureDomain if the platform type is VSphere.
func (f failureDomain) VSphere() machinev1.VSphereFailureDomain {
	return f.vsphere
}

// Nutanix returns the NutanixFailureDomainReference if the platform type is Nutanix.
func (f failureDomain) Nutanix() machinev1.NutanixFailureDomainReference {
	return f.nutanix
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
	case configv1.OpenStackPlatformType:
		return reflect.DeepEqual(f.openstack, other.OpenStack())
	case configv1.VSpherePlatformType:
		return reflect.DeepEqual(f.vsphere, other.VSphere())
	case configv1.NutanixPlatformType:
		return reflect.DeepEqual(f.nutanix, other.Nutanix())
	}

	return true
}

// CompleteFailureDomains calls Complete on each failure domain in the list.
func CompleteFailureDomains(failureDomains []FailureDomain, templateFailureDomain FailureDomain) ([]FailureDomain, error) {
	comparableFailureDomains := []FailureDomain{}

	for _, failureDomain := range failureDomains {
		failureDomain, err := failureDomain.Complete(templateFailureDomain)
		if err != nil {
			return nil, fmt.Errorf("cannot combine failure domain with template failure domain: %w", err)
		}

		comparableFailureDomains = append(comparableFailureDomains, failureDomain)
	}

	return comparableFailureDomains, nil
}

// Complete creates a copy of templateFailureDomain and overrides any set values with the values from the current failure domain.
func (f failureDomain) Complete(templateFailureDomain FailureDomain) (FailureDomain, error) {
	if templateFailureDomain == nil {
		return nil, errMissingTemplateFailureDomain
	}

	if f.platformType != templateFailureDomain.Type() {
		return nil, errMismatchedPlatformType
	}

	switch f.platformType {
	case configv1.AWSPlatformType:
		return f.completeAWS(templateFailureDomain.AWS()), nil
	case configv1.AzurePlatformType:
		return f.completeAzure(templateFailureDomain.Azure()), nil
	case configv1.GCPPlatformType:
		return f.completeGCP(templateFailureDomain.GCP()), nil
	case configv1.OpenStackPlatformType:
		return f.completeOpenStack(templateFailureDomain.OpenStack()), nil
	case configv1.VSpherePlatformType:
		return f.completeVSphere(templateFailureDomain.VSphere()), nil
	case configv1.NutanixPlatformType:
		return f.completeNutanix(templateFailureDomain.Nutanix()), nil
	default:
		return NewGenericFailureDomain(), nil
	}
}

func (f failureDomain) completeAWS(templateFailureDomain machinev1.AWSFailureDomain) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.aws.Placement.AvailabilityZone != "" {
		fd.Placement = f.aws.Placement
	}

	if f.aws.Subnet != nil && !reflect.DeepEqual(f.aws.Subnet, machinev1.AWSResourceReference{}) {
		fd.Subnet = f.aws.Subnet
	}

	return NewAWSFailureDomain(*fd)
}

func (f failureDomain) completeAzure(templateFailureDomain machinev1.AzureFailureDomain) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.azure.Zone != "" {
		fd.Zone = f.azure.Zone
	}

	if f.azure.Subnet != "" {
		fd.Subnet = f.azure.Subnet
	}

	return NewAzureFailureDomain(*fd)
}

func (f failureDomain) completeGCP(templateFailureDomain machinev1.GCPFailureDomain) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.gcp.Zone != "" {
		fd.Zone = f.gcp.Zone
	}

	return NewGCPFailureDomain(*fd)
}

func (f failureDomain) completeOpenStack(templateFailureDomain machinev1.OpenStackFailureDomain) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.openstack.AvailabilityZone != "" {
		fd.AvailabilityZone = f.openstack.AvailabilityZone
	}

	if fd.RootVolume == nil {
		fd.RootVolume = f.openstack.RootVolume
	} else if f.openstack.RootVolume != nil {
		if f.openstack.RootVolume.AvailabilityZone != "" {
			fd.RootVolume.AvailabilityZone = f.openstack.RootVolume.AvailabilityZone
		}

		if f.openstack.RootVolume.VolumeType != "" {
			fd.RootVolume.VolumeType = f.openstack.RootVolume.VolumeType
		}
	}

	return NewOpenStackFailureDomain(*fd)
}

func (f failureDomain) completeVSphere(templateFailureDomain machinev1.VSphereFailureDomain) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.vsphere.Name != "" {
		fd.Name = f.vsphere.Name
	}

	return NewVSphereFailureDomain(*fd)
}

func (f failureDomain) completeNutanix(templateFailureDomain machinev1.NutanixFailureDomainReference) FailureDomain {
	fd := templateFailureDomain.DeepCopy()

	if f.nutanix.Name != "" {
		fd.Name = f.nutanix.Name
	}

	return NewNutanixFailureDomain(*fd)
}

// NewFailureDomains creates a set of FailureDomains representing the input failure
// domains held within the ControlPlaneMachineSet.
func NewFailureDomains(failureDomains *machinev1.FailureDomains) ([]FailureDomain, error) {
	if failureDomains == nil {
		// Without failure domains all machines will be equal.
		return nil, nil
	}

	switch failureDomains.Platform {
	case configv1.AWSPlatformType:
		return newAWSFailureDomains(*failureDomains)
	case configv1.AzurePlatformType:
		return newAzureFailureDomains(*failureDomains)
	case configv1.GCPPlatformType:
		return newGCPFailureDomains(*failureDomains)
	case configv1.OpenStackPlatformType:
		return newOpenStackFailureDomains(*failureDomains)
	case configv1.VSpherePlatformType:
		return newVSphereFailureDomains(*failureDomains)
	case configv1.NutanixPlatformType:
		return newNutanixFailureDomains(*failureDomains)
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

// newOpenStackFailureDomains constructs a slice of OpenStack FailureDomain from machinev1.FailureDomains.
func newOpenStackFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}

	if len(failureDomains.OpenStack) == 0 {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, failureDomain := range failureDomains.OpenStack {
		foundFailureDomains = append(foundFailureDomains, NewOpenStackFailureDomain(failureDomain))
	}

	return foundFailureDomains, nil
}

// newVSphereFailureDomains constructs a slice of VSphere FailureDomain from machinev1.FailureDomains.
func newVSphereFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}

	if len(failureDomains.VSphere) == 0 {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, failureDomain := range failureDomains.VSphere {
		foundFailureDomains = append(foundFailureDomains, NewVSphereFailureDomain(failureDomain))
	}

	return foundFailureDomains, nil
}

// newNutanixFailureDomains constructs a slice of Nutanix FailureDomain from machinev1.FailureDomains.
func newNutanixFailureDomains(failureDomains machinev1.FailureDomains) ([]FailureDomain, error) {
	foundFailureDomains := []FailureDomain{}

	if len(failureDomains.Nutanix) == 0 {
		return foundFailureDomains, errMissingFailureDomain
	}

	for _, fdRef := range failureDomains.Nutanix {
		foundFailureDomains = append(foundFailureDomains, NewNutanixFailureDomain(fdRef))
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

// NewOpenStackFailureDomain creates an OpenStack failure domain from the machinev1.OpenStackFailureDomain.
func NewOpenStackFailureDomain(fd machinev1.OpenStackFailureDomain) FailureDomain {
	return &failureDomain{
		platformType: configv1.OpenStackPlatformType,
		openstack:    fd,
	}
}

// NewVSphereFailureDomain creates an VSphere failure domain from the machinev1.VSphereFailureDomain.
func NewVSphereFailureDomain(fd machinev1.VSphereFailureDomain) FailureDomain {
	return &failureDomain{
		platformType: configv1.VSpherePlatformType,
		vsphere:      fd,
	}
}

// NewNutanixFailureDomain creates an Nutanix failure domain from the machinev1.NutanixFailureDomainReference.
func NewNutanixFailureDomain(fdRef machinev1.NutanixFailureDomainReference) FailureDomain {
	return &failureDomain{
		platformType: configv1.NutanixPlatformType,
		nutanix:      fdRef,
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
	var failureDomain []string

	if fd.Zone != "" {
		failureDomain = append(failureDomain, fmt.Sprintf("Zone:%s", fd.Zone))
	}

	if fd.Subnet != "" {
		failureDomain = append(failureDomain, fmt.Sprintf("Subnet:%s", fd.Subnet))
	}

	if len(failureDomain) == 0 {
		return unknownFailureDomain
	}

	return "AzureFailureDomain{" + strings.Join(failureDomain, ", ") + "}"
}

// gcpFailureDomainToString converts the GCPFailureDomain into a string.
func gcpFailureDomainToString(fd machinev1.GCPFailureDomain) string {
	if fd.Zone != "" {
		return fmt.Sprintf("GCPFailureDomain{Zone:%s}", fd.Zone)
	}

	return unknownFailureDomain
}

// openstackFailureDomainToString converts the OpenStackFailureDomain into a string.
func openstackFailureDomainToString(fd machinev1.OpenStackFailureDomain) string {
	if fd.AvailabilityZone == "" && fd.RootVolume == nil {
		return unknownFailureDomain
	}

	var failureDomain []string

	if fd.AvailabilityZone != "" {
		failureDomain = append(failureDomain, "AvailabilityZone:"+fd.AvailabilityZone)
	}

	if fd.RootVolume != nil {
		var rootVolume []string

		if fd.RootVolume.AvailabilityZone != "" {
			rootVolume = append(rootVolume, "AvailabilityZone:"+fd.RootVolume.AvailabilityZone)
		}

		if fd.RootVolume.VolumeType != "" {
			rootVolume = append(rootVolume, "VolumeType:"+fd.RootVolume.VolumeType)
		}

		failureDomain = append(failureDomain, "RootVolume:{"+strings.Join(rootVolume, ", ")+"}")
	}

	return "OpenStackFailureDomain{" + strings.Join(failureDomain, ", ") + "}"
}

// vsphereFailureDomainToString converts the VSphereFailureDomain into a string.
func vsphereFailureDomainToString(fd machinev1.VSphereFailureDomain) string {
	if fd.Name != "" {
		return fmt.Sprintf("VSphereFailureDomain{Name:%s}", fd.Name)
	}

	return unknownFailureDomain
}

// nutanixFailureDomainToString converts the NutanixFailureDomainReference into a string.
func nutanixFailureDomainToString(fdRef machinev1.NutanixFailureDomainReference) string {
	if fdRef.Name != "" {
		return fmt.Sprintf("NutanixFailureDomainReference{Name:%s}", fdRef.Name)
	}

	return unknownFailureDomain
}

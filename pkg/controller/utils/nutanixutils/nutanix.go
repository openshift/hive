package nutanixutils

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/apis/hive/v1/nutanix"
	nutanixinstaller "github.com/openshift/installer/pkg/types/nutanix"
)

// convertFailureDomains is a generic function to convert failure domains between Hive and Installer formats and returns unique PrismElements and SubnetUUIDs.
//
// This function takes a slice of source failure domains and conversion functions for PrismElements,
// StorageResources, SubnetUUIDs, and the FailureDomain itself. It iterates through the source failure domains,
// converts their components using the provided functions, deduplicates subnet UUIDs and PrismElements, and
// returns a slice of converted target failure domains along with unique PrismElements and SubnetUUIDs.
//
// Type parameters:
//   - SourceFD: The type of the source failure domain.
//   - TargetFD: The type of the target failure domain.
//   - TargetPE: The type of the target PrismElement.
//   - SRR:      The type of the StorageResourceReference in the target format.
//
// Parameters:
//   - sourceFailureDomains: A slice of source failure domains to convert.
//   - convertPrismElement:    A function to convert a source failure domain's PrismElement to the target PrismElement type.
//   - convertStorageResource: A function to convert a source failure domain's StorageResources to slices of target StorageResourceReference types (storage containers and data source images).
//   - convertSubnetUUIDs:   A function to extract subnet UUIDs from a source failure domain.
//   - convertFailureDomain:   A function to construct a target failure domain from a source failure domain and its converted components.
//
// Returns:
//   - []TargetFD: A slice of converted target failure domains.
//   - []TargetPE: A slice of unique converted target PrismElements.
//   - []string:   A slice of unique subnet UUIDs gathered from all source failure domains.
func convertFailureDomains[SourceFD any, TargetFD any, TargetPE any, SRR any](
	sourceFailureDomains []SourceFD,
	convertPrismElement func(SourceFD) (TargetPE, error),
	convertStorageResource func(SourceFD) ([]SRR, []SRR),
	convertSubnetUUIDs func(any) []string,
	convertFailureDomain func(SourceFD, TargetPE, []SRR, []SRR) TargetFD,
) ([]TargetFD, []TargetPE, []string) {

	prismElements := make([]TargetPE, 0)
	subnetUUIDSet := sets.Set[string]{}
	failureDomains := make([]TargetFD, 0)
	prismElementMap := make(map[string]TargetPE)

	for _, failureDomain := range sourceFailureDomains {
		storageContainers, dataSourceImages := convertStorageResource(failureDomain)

		// Merge subnet UUIDs using sets for efficient deduplication
		for _, uuid := range convertSubnetUUIDs(failureDomain) {
			subnetUUIDSet.Insert(uuid)
		}

		// Ensure PrismElements are unique by UUID
		prismElement, err := convertPrismElement(failureDomain)
		prismElementKey := getPrismElementUUID(failureDomain)

		if _, exists := prismElementMap[prismElementKey]; !exists && err == nil {
			prismElementMap[prismElementKey] = prismElement
			prismElements = append(prismElements, prismElement)
		}

		// Append the converted failure domain
		failureDomains = append(failureDomains, convertFailureDomain(failureDomain, prismElement, storageContainers, dataSourceImages))
	}

	// Convert the set to a sorted list
	subnetUUIDs := subnetUUIDSet.UnsortedList() // Alternative: Use `subnetUUIDSet.List()` if sorting is needed

	return failureDomains, prismElements, subnetUUIDs
}

// ConvertHiveFailureDomains converts Hive failure domains to Installer failure domains and returns unique PrismElements and SubnetUUIDs.
//
// This function specializes the generic convertFailureDomains function to convert
// Hive's nutanix.FailureDomain type to Installer's nutanixinstaller.FailureDomain type.
// It uses specific conversion functions for PrismElement, StorageResource, and FailureDomain
// tailored for the Hive to Installer conversion. It also returns lists of unique PrismElements and SubnetUUIDs.
//
// Parameters:
//   - hiveFailureDomains: A slice of Hive failure domains (nutanix.FailureDomain) to convert.
//
// Returns:
//   - []nutanixinstaller.FailureDomain: A slice of converted Installer failure domains (nutanixinstaller.FailureDomain).
//   - []nutanixinstaller.PrismElement:  A slice of unique converted Installer PrismElements (nutanixinstaller.PrismElement).
//   - []string:                       A slice of unique subnet UUIDs gathered from all Hive failure domains.
func ConvertHiveFailureDomains(hiveFailureDomains []nutanix.FailureDomain) ([]nutanixinstaller.FailureDomain, []nutanixinstaller.PrismElement, []string) {
	return convertFailureDomains(
		hiveFailureDomains,
		convertHiveToInstallerPrismElement,
		convertHiveToInstallerStorageResource,
		getSubnetUUIDs,
		convertHiveToInstallerFailureDomain,
	)
}

// ConvertInstallerFailureDomains converts Installer failure domains to Hive failure domains and returns unique PrismElements and SubnetUUIDs.
//
// This function specializes the generic convertFailureDomains function to convert
// Installer's nutanixinstaller.FailureDomain type to Hive's nutanix.FailureDomain type.
// It uses specific conversion functions for PrismElement, StorageResource, and FailureDomain
// tailored for the Installer to Hive conversion. It also returns lists of unique PrismElements and SubnetUUIDs.
//
// Parameters:
//   - installerFailureDomains: A slice of Installer failure domains (nutanixinstaller.FailureDomain) to convert.
//
// Returns:
//   - []nutanix.FailureDomain:    A slice of converted Hive failure domains (nutanix.FailureDomain).
//   - []nutanix.PrismElement:     A slice of unique converted Hive PrismElements (nutanix.PrismElement).
//   - []string:                  A slice of unique subnet UUIDs gathered from all Installer failure domains.
func ConvertInstallerFailureDomains(installerFailureDomains []nutanixinstaller.FailureDomain) ([]nutanix.FailureDomain, []nutanix.PrismElement, []string) {
	return convertFailureDomains(
		installerFailureDomains,
		convertInstallerToHivePrismElement,
		convertInstallerToHiveStorageResource,
		getSubnetUUIDs,
		convertInstallerToHiveFailureDomain,
	)
}

// convertHiveToInstallerPrismElement converts a Hive PrismElement to an Installer PrismElement.
// Returns:
//   - nutanixinstaller.PrismElement: The converted Installer PrismElement.
func convertHiveToInstallerPrismElement(failureDomain nutanix.FailureDomain) (nutanixinstaller.PrismElement, error) {
	if failureDomain.PrismElement.UUID == "" {
		return nutanixinstaller.PrismElement{}, errors.New("no prism element found for hive failure domain " + failureDomain.Name)
	}

	return nutanixinstaller.PrismElement{
		UUID: failureDomain.PrismElement.UUID,
		Endpoint: nutanixinstaller.PrismEndpoint{
			Address: failureDomain.PrismElement.Endpoint.Address,
			Port:    failureDomain.PrismElement.Endpoint.Port,
		},
		Name: failureDomain.PrismElement.Name,
	}, nil
}

// convertInstallerToHivePrismElement converts an Installer PrismElement to a Hive PrismElement.
// Returns:
//   - nutanix.PrismElement: The converted Hive PrismElement.
func convertInstallerToHivePrismElement(failureDomain nutanixinstaller.FailureDomain) (nutanix.PrismElement, error) {
	if failureDomain.PrismElement.UUID == "" {
		return nutanix.PrismElement{}, errors.New("no prism element found for installer failure domain " + failureDomain.Name)
	}

	return nutanix.PrismElement{
		UUID: failureDomain.PrismElement.UUID,
		Endpoint: nutanix.PrismEndpoint{
			Address: failureDomain.PrismElement.Endpoint.Address,
			Port:    failureDomain.PrismElement.Endpoint.Port,
		},
		Name: failureDomain.PrismElement.Name,
	}, nil
}

// convertHiveToInstallerStorageResource converts Hive StorageResources to Installer StorageResourceReferences.
// Returns:
//   - []nutanixinstaller.StorageResourceReference: A slice of converted Installer StorageResourceReferences for StorageContainers.
//   - []nutanixinstaller.StorageResourceReference: A slice of converted Installer StorageResourceReferences for DataSourceImages.
func convertHiveToInstallerStorageResource(failureDomain nutanix.FailureDomain) ([]nutanixinstaller.StorageResourceReference, []nutanixinstaller.StorageResourceReference) {
	var storageContainers []nutanixinstaller.StorageResourceReference
	var dataSourceImages []nutanixinstaller.StorageResourceReference

	for _, storageContainer := range failureDomain.StorageContainers {
		storageContainers = append(storageContainers, nutanixinstaller.StorageResourceReference{
			ReferenceName: storageContainer.ReferenceName,
			UUID:          storageContainer.UUID,
			Name:          storageContainer.Name,
		})
	}

	for _, dataSourceImage := range failureDomain.DataSourceImages {
		dataSourceImages = append(dataSourceImages, nutanixinstaller.StorageResourceReference{
			ReferenceName: dataSourceImage.ReferenceName,
			UUID:          dataSourceImage.UUID,
			Name:          dataSourceImage.Name,
		})
	}

	return storageContainers, dataSourceImages
}

// convertInstallerToHiveStorageResource converts Installer StorageResources to Hive StorageResourceReferences.
// Returns:
//   - []nutanix.StorageResourceReference: A slice of converted Hive StorageResourceReferences for StorageContainers.
//   - []nutanix.StorageResourceReference: A slice of converted Hive StorageResourceReferences for DataSourceImages.
func convertInstallerToHiveStorageResource(failureDomain nutanixinstaller.FailureDomain) ([]nutanix.StorageResourceReference, []nutanix.StorageResourceReference) {
	var storageContainers []nutanix.StorageResourceReference
	var dataSourceImages []nutanix.StorageResourceReference

	for _, storageContainer := range failureDomain.StorageContainers {
		storageContainers = append(storageContainers, nutanix.StorageResourceReference{
			ReferenceName: storageContainer.ReferenceName,
			UUID:          storageContainer.UUID,
			Name:          storageContainer.Name,
		})
	}

	for _, dataSourceImage := range failureDomain.DataSourceImages {
		dataSourceImages = append(dataSourceImages, nutanix.StorageResourceReference{
			ReferenceName: dataSourceImage.ReferenceName,
			UUID:          dataSourceImage.UUID,
			Name:          dataSourceImage.Name,
		})
	}

	return storageContainers, dataSourceImages
}

// getSubnetUUIDs extracts Subnet UUIDs from a FailureDomain.
// Returns:
//   - []string: A slice of Subnet UUID strings from the given failure domain.
func getSubnetUUIDs(failureDomain any) []string {
	switch v := failureDomain.(type) {
	case nutanix.FailureDomain:
		return v.SubnetUUIDs
	case nutanixinstaller.FailureDomain:
		return v.SubnetUUIDs
	default:
		return nil
	}
}

// getPrismElementUUID extracts the PrismElement UUID from a FailureDomain.
// Returns:
//   - string: The UUID of the PrismElement, or an empty string if the type is not recognized.
func getPrismElementUUID(failureDomain any) string {
	switch v := failureDomain.(type) {
	case nutanix.FailureDomain:
		return v.PrismElement.UUID
	case nutanixinstaller.FailureDomain:
		return v.PrismElement.UUID
	default:
		return ""
	}
}

// convertHiveToInstallerFailureDomain constructs an Installer FailureDomain from a Hive FailureDomain and its converted components.
// Returns:
//   - nutanixinstaller.FailureDomain: The constructed Installer failure domain.
func convertHiveToInstallerFailureDomain(
	failureDomain nutanix.FailureDomain,
	prismElement nutanixinstaller.PrismElement,
	storageContainers []nutanixinstaller.StorageResourceReference,
	dataSourceImages []nutanixinstaller.StorageResourceReference,
) nutanixinstaller.FailureDomain {
	return nutanixinstaller.FailureDomain{
		Name:              failureDomain.Name,
		PrismElement:      prismElement,
		SubnetUUIDs:       failureDomain.SubnetUUIDs,
		StorageContainers: storageContainers,
		DataSourceImages:  dataSourceImages,
	}
}

// convertInstallerToHiveFailureDomain constructs a Hive FailureDomain from an Installer FailureDomain and its converted components.
// Returns:
//   - nutanix.FailureDomain: The constructed Hive failure domain.
func convertInstallerToHiveFailureDomain(
	failureDomain nutanixinstaller.FailureDomain,
	prismElement nutanix.PrismElement,
	storageContainers []nutanix.StorageResourceReference,
	dataSourceImages []nutanix.StorageResourceReference,
) nutanix.FailureDomain {
	return nutanix.FailureDomain{
		Name:              failureDomain.Name,
		PrismElement:      prismElement,
		SubnetUUIDs:       failureDomain.SubnetUUIDs,
		StorageContainers: storageContainers,
		DataSourceImages:  dataSourceImages,
	}
}

// ExtractInstallerResources extracts unique PrismElements and Subnet UUIDs from a slice of Installer failure domains.
// This function iterates through the provided failure domains, collecting unique PrismElements
// and Subnet UUIDs. It ensures deduplication using a set for Subnet UUIDs.
//
// Parameters:
//   - installerFailureDomains: A slice of Installer failure domains (nutanixinstaller.FailureDomain).
//
// Returns:
//   - []nutanixinstaller.PrismElement: A deduplicated list of PrismElements from all Installer failure domains.
//   - []string: A deduplicated list of Subnet UUIDs from all Installer failure domains.
func ExtractInstallerResources(installerFailureDomains []nutanixinstaller.FailureDomain) ([]nutanixinstaller.PrismElement, []string) {
	prismElements := []nutanixinstaller.PrismElement{}
	subnetUUIDSet := sets.Set[string]{}

	for _, fd := range installerFailureDomains {
		// Extract and deduplicate PrismElements
		if fd.PrismElement.UUID != "" {
			prismElements = append(prismElements, fd.PrismElement)
		}

		// Extract and deduplicate Subnet UUIDs
		for _, uuid := range fd.SubnetUUIDs {
			subnetUUIDSet.Insert(uuid)
		}
	}

	// Convert subnet UUIDs from set to slice
	return prismElements, subnetUUIDSet.UnsortedList()
}

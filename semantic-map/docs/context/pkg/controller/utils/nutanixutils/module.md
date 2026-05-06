# Module atlas

## Responsibility

Provides bidirectional conversion functions between Hive and OpenShift Installer Nutanix failure domain types, including PrismElement, StorageResource, and SubnetUUID extraction with deduplication. Used by controllers that need to translate Nutanix platform configuration between the two type systems.

## Public Interface/API

- `func ConvertHiveFailureDomains(hiveFailureDomains []nutanix.FailureDomain) ([]nutanixinstaller.FailureDomain, []nutanixinstaller.PrismElement, []string)` -- converts Hive failure domains to Installer format, returns unique PrismElements and SubnetUUIDs
- `func ConvertInstallerFailureDomains(installerFailureDomains []nutanixinstaller.FailureDomain) ([]nutanix.FailureDomain, []nutanix.PrismElement, []string)` -- converts Installer failure domains to Hive format, returns unique PrismElements and SubnetUUIDs
- `func ExtractInstallerResources(installerFailureDomains []nutanixinstaller.FailureDomain) ([]nutanixinstaller.PrismElement, []string)` -- extracts unique PrismElements and SubnetUUIDs from Installer failure domains

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1/nutanix` -- Hive Nutanix failure domain, PrismElement, StorageResourceReference types
- `github.com/openshift/installer/pkg/types/nutanix` -- Installer Nutanix failure domain, PrismElement, StorageResourceReference types
- `github.com/pkg/errors` -- error wrapping
- `k8s.io/apimachinery/pkg/util/sets` -- set-based deduplication for SubnetUUIDs

## Capabilities

- Generic internal `convertFailureDomains` function using Go generics for bidirectional conversion
- Converts PrismElement, StorageContainers, and DataSourceImages between Hive and Installer type systems
- Deduplicates PrismElements by UUID and SubnetUUIDs using sets
- Extracts unique infrastructure resources from Installer failure domains without conversion

## Understanding Score

0.90

# Module atlas

## Responsibility

Small utility package for converting Nutanix failure domain types between Hive and Installer representations and extracting unique Prism Elements and subnet UUIDs.

## Public Interface/API

- `ConvertHiveFailureDomains` -- Converts Hive Nutanix failure domains to Installer failure domains, returning unique PrismElements and SubnetUUIDs.
- `ConvertInstallerFailureDomains` -- Converts Installer Nutanix failure domains to Hive failure domains, returning unique PrismElements and SubnetUUIDs.
- `ExtractInstallerResources` -- Extracts unique PrismElements and Subnet UUIDs from Installer failure domains.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1/nutanix` -- Hive Nutanix failure domain types.
- `github.com/openshift/installer/pkg/types/nutanix` -- Installer Nutanix failure domain types.

## Capabilities

Pure utility package. Bidirectional conversion between Hive and Installer Nutanix failure domain representations. Used by the machinepool controller's Nutanix actuator and the clusterpool controller.

## Understanding Score

0.90

# Module atlas

## Responsibility

Small utility package for converting deprecated vSphere fields on ClusterDeployment specs to the newer Infrastructure-based format.

## Public Interface/API

- `ConvertDeprecatedFields` -- Converts deprecated vSphere fields (DeprecatedVCenter, DeprecatedFolder, etc.) to the new `Infrastructure.VCenters` and `Infrastructure.FailureDomains` format using the installer's conversion utilities.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1/vsphere` -- Hive vSphere platform types.
- `github.com/openshift/installer/pkg/types/vsphere` -- Installer vSphere types.
- `github.com/openshift/installer/pkg/types/vsphere/conversion` -- Installer conversion utilities.

## Capabilities

Pure utility package. Handles the migration path from the old single-vCenter vSphere configuration to the newer multi-vCenter Infrastructure-based format. Used by the clusterdeployment controller during reconciliation of legacy CDs.

## Understanding Score

0.90

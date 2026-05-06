# Module atlas

## Responsibility

Defines the `Actuator` interface and supporting types for cloud-specific private link operations, used by the privatelink controller to abstract over AWS PrivateLink and GCP Private Service Connect.

## Public Interface/API

- `Actuator` -- interface:
  - `Cleanup(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata, logger) error`
  - `CleanupRequired(cd *hivev1.ClusterDeployment) bool`
  - `Reconcile(cd, metadata, *DnsRecord, logger) (reconcile.Result, error)`
  - `ShouldSync(cd *hivev1.ClusterDeployment) bool`
- `ActuatorType` -- string type with constants `ActuatorTypeHub` and `ActuatorTypeLink`
- `DnsRecord` -- struct with `IpAddress []string` and `AliasTarget`
- `AliasTarget` -- struct with `Name string` and `HostedZoneID string`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/sirupsen/logrus`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`

## Capabilities

- Provides the contract for private link actuator implementations (AWS, GCP)
- Defines DNS record types used to configure private link API endpoint DNS records
- Distinguishes between Hub (management cluster side) and Link (spoke cluster side) actuator types

## Understanding Score

0.9

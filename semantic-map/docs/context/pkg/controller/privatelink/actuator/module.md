# Module atlas

## Responsibility

Defines the `Actuator` interface for private link cloud-specific implementations and supporting types. This is a pure interface/types package with no implementation.

## Public Interface/API

- `Actuator` -- Interface: `Cleanup(cd, metadata, logger)`, `CleanupRequired(cd)`, `Reconcile(cd, metadata, dnsRecord, logger)`, `ShouldSync(cd)`.
- `ActuatorType` -- String type with constants `ActuatorTypeHub` and `ActuatorTypeLink`.
- `DnsRecord` -- Struct with `IpAddress []string` and `AliasTarget`.
- `AliasTarget` -- Struct with `Name` and `HostedZoneID` for DNS alias records.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterMetadata types.
- `sigs.k8s.io/controller-runtime/pkg/reconcile` -- reconcile.Result.

## Capabilities

Interface-only package defining the contract for private link actuator implementations.

## Understanding Score

0.90

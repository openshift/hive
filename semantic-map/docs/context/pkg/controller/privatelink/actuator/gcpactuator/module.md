# Module atlas

## Responsibility

Provides the GCP implementation of the private link actuator interface. Manages GCP Private Service Connect resources including forwarding rules, service attachments, and DNS records for private cluster API access.

## Public Interface/API

- `GCPLinkActuator` -- GCP implementation of the `actuator.Actuator` interface.
- `GCPLinkActuator.Reconcile` -- Creates/manages GCP Private Service Connect resources.
- `GCPLinkActuator.Cleanup` -- Removes GCP private link resources.
- `GCPLinkActuator.CleanupRequired` -- Checks if cleanup is needed.
- `GCPLinkActuator.ShouldSync` -- Rate-limits cloud API calls.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterMetadata types.
- `github.com/openshift/hive/apis/hive/v1/gcp` -- GCP-specific PrivateLink configuration types.
- `github.com/openshift/hive/pkg/gcpclient` -- GCP client wrapper.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface and types.
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- Condition management.
- `github.com/openshift/hive/pkg/controller/utils` -- Controller utilities.
- `google.golang.org/api/compute/v1` -- GCP Compute API for forwarding rules, service attachments.

## Capabilities

- Implements: `actuator.Actuator` interface.
- Key logic: Manages GCP Private Service Connect forwarding rules and service attachments for private API access. Creates DNS records pointing to the private endpoint.

## Understanding Score

0.80

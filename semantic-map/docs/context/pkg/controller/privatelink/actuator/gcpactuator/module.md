# Module atlas

## Responsibility

Implements the private link Actuator interface for GCP, managing Private Service Connect (PSC) resources including service attachments, forwarding rules, subnets, and endpoint connections between Hive's management cluster and spoke clusters.

## Public Interface/API

- `GCPLinkActuator` -- implements `actuator.Actuator` for GCP private service connect
  - `Cleanup(cd, metadata, logger) error` -- cleans up PSC resources (forwarding rules, subnets, service attachments)
  - `CleanupRequired(cd) bool`
  - `Reconcile(cd, metadata, dnsRecord, logger) (reconcile.Result, error)` -- reconciles PSC resources
  - `ShouldSync(cd) bool`
- `NewGCPLinkActuator(client, config, cd, privateLinkEnabled, gcpClientFn, logger) (*GCPLinkActuator, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `apis/hive/v1/gcp` -- GCP-specific status types
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- condition helpers
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/gcpclient` -- GCP client factory
- `google.golang.org/api/compute/v1` -- GCP Compute API

## Capabilities

- Manages GCP Private Service Connect lifecycle: service attachments, forwarding rules, subnets
- Creates hub and spoke GCP clients from Kubernetes secrets
- Updates `PrivateServiceConnect` status on ClusterDeployment
- Handles cleanup of PSC resources on deletion, with retry-on-conflict for status updates
- Uses `defaultServiceAttachmentSubnetCidr` for subnet allocation
- Detects 404 errors from GCP API for idempotent cleanup

## Understanding Score

0.85

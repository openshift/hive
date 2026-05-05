# Module atlas

## Responsibility

Provides a controller that manages DNSZone objects, creating and maintaining DNS hosted zones in cloud providers. Uses an actuator pattern with cloud-specific implementations for AWS Route53, Azure DNS, and GCP Cloud DNS. The controller handles zone creation, deletion, metadata updates, name server discovery, and SOA record validation.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `ControllerName` -- Constant `"dnszone"`.
- `Actuator` -- Interface: `Create`, `Delete`, `Exists`, `Refresh`, `GetNameServers`, `UpdateMetadata`, `SetConditionsForError`.
- `AWSActuator` -- AWS Route53 implementation. Manages hosted zones with tag-based discovery.
- `AzureActuator` -- Azure DNS implementation. Manages DNS zones in resource groups.
- `GCPActuator` -- GCP Cloud DNS implementation. Manages managed zones.
- `ReconcileDNSZone` -- Main reconciler struct.
- `ReconcileDNSZone.Reconcile` -- Creates/deletes/updates DNS zones via actuators, syncs name servers to status.
- `DeleteAWSRecordSets` / `DeleteAzureRecordSets` / `DeleteGCPRecordSets` -- Clean up non-essential records before zone deletion.
- `IsErrorUpdateEvent` -- Predicate for rate-limited error event handling.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- DNSZone CR.
- `github.com/openshift/hive/pkg/awsclient` -- AWS client for Route53 operations.
- `github.com/openshift/hive/pkg/azureclient` -- Azure client for DNS zone operations.
- `github.com/openshift/hive/pkg/gcpclient` -- GCP client for Cloud DNS operations.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, finalizers, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **DNSZone**.
- Watches: DNSZone (with rate-limited error update handler).
- Conditions set: Various cloud-specific error conditions on the DNSZone.
- Key logic: Uses actuator pattern -- selects cloud actuator based on DNSZone spec (AWS/Azure/GCP). Creates hosted zone if not exists, refreshes state, updates metadata (tags), retrieves name servers into status. On deletion, cleans up record sets then deletes the zone. Validates SOA records for zone availability.
- Metrics: `hive_dnszone_reconcile_duration_seconds`.

## Understanding Score

0.80

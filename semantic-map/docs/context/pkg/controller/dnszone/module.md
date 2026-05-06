# Module atlas

## Responsibility

Reconciles DNSZone custom resources by managing hosted zones in cloud DNS providers (AWS Route53, Azure DNS, GCP Cloud DNS). Creates, refreshes, updates metadata, and deletes DNS zones via a cloud-specific Actuator pattern, and syncs name server status back to the DNSZone resource.

## Public Interface/API

- `ControllerName` -- constant `hivev1.DNSZoneControllerName`
- `Add(mgr manager.Manager) error` -- registers controller with watches on DNSZone and ClusterDeployment
- `Actuator` -- interface: `Create()`, `Delete()`, `Exists()`, `UpdateMetadata()`, `GetNameServers()`, `Refresh()`, `SetConditionsForError(err) bool`
- `AWSActuator` -- implements Actuator for AWS Route53 hosted zones
- `AzureActuator` -- implements Actuator for Azure DNS managed zones
- `GCPActuator` -- implements Actuator for GCP Cloud DNS managed zones
- `ReconcileDNSZone` -- reconciler struct
- `ReconcileDNSZone.Reconcile(ctx, request) (reconcile.Result, error)`
- `DeleteAWSRecordSets(...)` -- cleans up DNS zone to minimum required records (AWS)
- `DeleteAzureRecordSets(...)` -- removes non-essential records (Azure)
- `DeleteGCPRecordSets(...)` -- deletes non-essential records (GCP)
- `IsErrorUpdateEvent(event.UpdateEvent) bool` -- predicate for rate-limited event handling

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient` -- cloud client factories
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, conditions, finalizers
- `github.com/miekg/dns` -- DNS SOA lookups for zone availability checks
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, manager

## Capabilities

- Creates and manages hosted zones in AWS Route53, Azure DNS, and GCP Cloud DNS
- Refreshes zone state from cloud provider on each reconcile
- Updates zone metadata (tags) to match DNSZone spec
- Handles zone deletion with record set cleanup before zone removal
- Verifies zone availability via SOA record lookups
- Sets conditions for access denied, authentication failure, API opt-in, and cloud errors
- Tracks deleted DNS zones via Prometheus counter metric

## Understanding Score

0.85

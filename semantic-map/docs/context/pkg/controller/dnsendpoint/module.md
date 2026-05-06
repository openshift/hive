# Module atlas

## Responsibility

Reconciles DNSZone objects to maintain NS records for subdomains in their parent (root) managed domain hosted zones. Periodically scrapes cloud DNS providers to detect and correct name server discrepancies, ensuring subdomain delegation is consistent with DNSZone status.

## Public Interface/API

- `ControllerName` -- constant `hivev1.DNSEndpointControllerName`
- `Add(mgr manager.Manager) error` -- creates controllers for DNSZone watches and per-root-domain nameServerScrapers
- `ReconcileDNSEndpoint` -- reconciler struct embedding `client.Client`
- `ReconcileDNSEndpoint.Reconcile(ctx, request) (reconcile.Result, error)` -- syncs subdomain NS records in root domain hosted zones, handles create/update/delete and finalizer lifecycle

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver` -- Query interface for cloud DNS operations
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, finalizers, conditions, client wrappers
- `github.com/openshift/hive/pkg/manageddns` -- reads managed domains config file
- `github.com/prometheus/client_golang/prometheus` -- scrape metrics
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, manager, watches

## Capabilities

- Watches DNSZone and ClusterDeployment resources
- Creates one nameServerScraper per root managed domain that periodically queries cloud DNS and caches NS records
- On reconcile, compares scraped NS cache with DNSZone.Status.NameServers and creates/updates/deletes NS records via the Query interface
- Manages `FinalizerDNSEndpoint` finalizer for cleanup on deletion
- Sets `ParentLinkCreated` and `DomainNotManaged` conditions on DNSZone
- Exposes Prometheus metrics for scrape duration and subdomain counts

## Understanding Score

0.85

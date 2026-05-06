# Module atlas

## Responsibility

Periodically checks whether an installed remote cluster is reachable via its API server and maintains the `Unreachable` and `ActiveAPIURLOverride` conditions on ClusterDeployments. When a cluster is unreachable, other controllers can skip remote API calls that would otherwise incur a 30-second timeout. Supports primary and secondary (override) API URLs with fallback logic.

## Public Interface/API

- `const ControllerName` -- hivev1.UnreachableControllerName
- `func Add(mgr manager.Manager) error` -- creates and registers the controller
- `func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler` -- returns a new reconciler with remote client builder
- `func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error` -- registers controller watching ClusterDeployments
- `type ReconcileRemoteMachineSet struct` -- reconciler; embeds client.Client
- `func (r *ReconcileRemoteMachineSet) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)` -- checks API reachability and updates conditions

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, condition types
- `github.com/openshift/hive/pkg/constants` -- annotation keys
- `github.com/openshift/hive/pkg/controller/metrics` -- reconcile time observer
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, client wrapper, conditions, logging
- `github.com/openshift/hive/pkg/remoteclient` -- Builder for remote API client, Unreachable/SetUnreachableCondition, IsPrimaryURLActive
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, client, handler, source

## Capabilities

- Watches ClusterDeployments and reconciles only installed clusters with metadata
- Attempts connection to remote cluster using primary API URL, falls back to secondary API URL override
- Sets `Unreachable` condition (true/false) based on connection success
- Sets `ActiveAPIURLOverride` condition when an API URL override is configured
- Re-checks connectivity every 2 hours for reachable clusters
- Uses backoff-based requeue for unreachable clusters
- Respects reconcile pause annotation
- Initializes controller-specific conditions on first reconcile

## Understanding Score

0.85

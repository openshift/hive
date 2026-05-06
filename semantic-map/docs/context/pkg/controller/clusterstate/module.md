# Module atlas

## Responsibility

Maintains ClusterState resources that reflect the state of cluster operators from remote installed clusters. Periodically polls the remote cluster API to update the ClusterState status with current ClusterOperator conditions.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterStateControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterState` — reconciler struct embedding `client.Client`
- `ReconcileClusterState.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/api/config/v1` — ClusterOperator types from remote cluster
- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment, ClusterState CRDs
- `github.com/openshift/hive/pkg/constants` — pause annotation
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, owner references
- `github.com/openshift/hive/pkg/remoteclient` — remote cluster client builder
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterDeployment resources
- Creates and owns ClusterState resources for installed ClusterDeployments
- Connects to remote cluster APIs to read ClusterOperator status
- Updates ClusterState status at a 10-minute interval
- Manages owner references so ClusterState is garbage collected with the ClusterDeployment
- Skips reconciliation for uninstalled clusters or those with pause annotation

## Understanding Score

0.85

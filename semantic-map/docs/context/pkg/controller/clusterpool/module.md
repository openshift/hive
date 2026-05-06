# Module atlas

## Responsibility

Manages ClusterPool resources by maintaining a pool of pre-provisioned ClusterDeployments at the desired size, handling cluster claims and assignments, managing pool namespaces, and enforcing pool capacity limits. Supports hibernation/running state transitions and stale cluster replacement.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterpoolControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *ReconcileClusterPool`
- `AddToManager(mgr manager.Manager, r *ReconcileClusterPool, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterPool` — reconciler struct embedding `client.Client` with expectations cache
- `ReconcileClusterPool.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `github.com/openshift/hive/apis/helpers` — ClusterPool, ClusterDeployment, ClusterClaim CRDs
- `github.com/openshift/hive/pkg/clusterresource` — cluster resource generation
- `github.com/openshift/hive/pkg/constants` — label keys, constants
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, expectations
- `github.com/openshift/hive/pkg/controller/utils/vsphereutils` — vSphere validation utilities
- `github.com/openshift/hive/pkg/util/yaml` — YAML patch utilities
- `github.com/davegardnerisme/deephash` — deep hashing for spec change detection
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client, metrics

## Capabilities

- Watches ClusterPool, ClusterDeployment, and ClusterClaim resources
- Indexes ClusterDeployments by pool namespace/name and ClusterClaims by pool name
- Creates ClusterDeployments to maintain pool size, with namespace isolation per cluster
- Assigns unclaimed clusters to pending ClusterClaims
- Creates hive-cluster-pool-admin Role and RoleBinding for pool RBAC
- Manages a finalizer (`hive.openshift.io/clusters`) on ClusterPools
- Detects and replaces stale clusters whose spec no longer matches the pool
- Manages pool conditions: MissingDependencies, CapacityAvailable, AllClustersCurrent, InventoryValid, DeletionPossible
- Emits Prometheus gauge metrics per pool: assignable, claimed, deleting, installing, unclaimed, standby, stale, broken cluster counts

## Understanding Score

0.85

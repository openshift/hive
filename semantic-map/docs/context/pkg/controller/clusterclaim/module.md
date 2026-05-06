# Module atlas

## Responsibility

Reconciles ClusterClaim resources, managing the lifecycle of claims against ClusterPools. Ensures claimed ClusterDeployments have proper RBAC (hive-claim-owner Role and RoleBinding) for the claiming subject, and tracks claim status conditions.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterClaimControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *ReconcileClusterClaim`
- `AddToManager(mgr manager.Manager, r *ReconcileClusterClaim, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterClaim` — reconciler struct embedding `client.Client`
- `ReconcileClusterClaim.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterClaim, ClusterDeployment, ClusterPool CRDs
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers
- `github.com/openshift/hive/pkg/resource` — resource helper
- `k8s.io/api/rbac/v1` — Role, RoleBinding
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterClaim, ClusterDeployment, Role, and RoleBinding resources
- Manages a finalizer (`hive.openshift.io/claim`) on ClusterClaims
- Creates hive-claim-owner Role and RoleBinding for claimed clusters
- Maps ClusterDeployment changes back to the associated ClusterClaim for reconciliation
- Manages `ClusterClaimPending` and `ClusterRunning` conditions on ClusterClaims

## Understanding Score

0.85

# Module atlas

## Responsibility

Applies SyncSets and SelectorSyncSets to remote clusters by reconciling ClusterDeployment resources. Manages the ClusterSync internal resource to track the status of each syncset application, supports templated resources with ClusterDeployment parameters, and periodically reapplies resources.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClustersyncControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*ReconcileClusterSync, error)`
- `AddToManager(mgr manager.Manager, r *ReconcileClusterSync, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterSync` — reconciler struct embedding `client.Client` with reapply interval and ordinal ID
- `ReconcileClusterSync.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`
- `CommonSyncSet` — interface with `AsRuntimeObject()`, `AsMetaObject()`, `GetSpec()` methods
- `SyncSetAsCommon` — `hivev1.SyncSet` typed as `CommonSyncSet`
- `SelectorSyncSetAsCommon` — `hivev1.SelectorSyncSet` typed as `CommonSyncSet`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — SyncSet, SelectorSyncSet, ClusterDeployment CRDs
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` — ClusterSync internal resource
- `github.com/openshift/hive/pkg/constants` — env vars (reapply interval)
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers, ordinal ID
- `github.com/openshift/hive/pkg/remoteclient` — remote cluster client builder
- `github.com/openshift/hive/pkg/resource` — resource helper for applying resources to remote clusters
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client, metrics

## Capabilities

- Watches ClusterDeployment, SyncSet, SelectorSyncSet, and ClusterSync resources
- Applies resources (apply, createOrUpdate, createOnly modes) and patches to remote clusters
- Supports text/template parameter substitution in resources and patches using ClusterDeployment data (including `fromCDLabel` function)
- Manages ClusterSync status to track per-syncset application state
- Reapplies resources at configurable intervals (default 2 hours with 10% jitter)
- Supports ordinal-based sharding for multi-replica deployments
- Emits Prometheus metrics: syncset apply duration, resource apply counts/duration, time to first full success

## Understanding Score

0.85

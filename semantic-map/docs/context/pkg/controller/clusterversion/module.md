# Module atlas

## Responsibility

Syncs the remote cluster's ClusterVersion status to the local ClusterDeployment status. Periodically polls the remote cluster to read the `version` ClusterVersion object and updates the local ClusterDeployment's version status fields.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterVersionControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterVersion` — reconciler struct embedding `client.Client`
- `ReconcileClusterVersion.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/api/config/v1` — ClusterVersion type from remote cluster
- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment CRD
- `github.com/openshift/hive/pkg/constants` — poll interval env var, pause annotation
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers
- `github.com/openshift/hive/pkg/remoteclient` — remote cluster client builder
- `github.com/blang/semver/v4` — semantic version parsing
- `github.com/google/go-cmp/cmp` — diff detection for status changes
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches ClusterDeployment resources
- Connects to remote cluster APIs to read the `version` ClusterVersion object
- Syncs remote ClusterVersion status (available updates, version history) to ClusterDeployment status
- Supports configurable poll interval via environment variable (default: no polling, only on CR changes)
- Skips reconciliation for uninstalled, deleted, or paused clusters

## Understanding Score

0.85

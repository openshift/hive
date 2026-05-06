# Module atlas

## Responsibility

Reaps namespaces created for ClusterPool clusters after all ClusterDeployments have been removed. Also cleans up previously claimed ClusterDeployments that are no longer needed.

## Public Interface/API

- `ControllerName` — constant (from `hivev1.ClusterpoolNamespaceControllerName`)
- `Add(mgr manager.Manager) error` — creates and registers the controller with the manager
- `NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler`
- `AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error`
- `ReconcileClusterPoolNamespace` — reconciler struct embedding `client.Client`
- `ReconcileClusterPoolNamespace.Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment CRD
- `github.com/openshift/hive/pkg/constants` — ClusterPoolNameLabel
- `github.com/openshift/hive/pkg/controller/metrics` — reconcile observer
- `github.com/openshift/hive/pkg/controller/utils` — controller config, client wrappers
- `sigs.k8s.io/controller-runtime` — controller, reconcile, manager, client

## Capabilities

- Watches Namespace and ClusterDeployment resources
- Deletes namespaces labeled with `hive.openshift.io/clusterpool-name` once they contain no ClusterDeployments
- Enforces a minimum namespace lifetime (5 minutes) before deletion
- Cleans up previously claimed ClusterDeployments within pool namespaces
- Maps ClusterDeployment changes to reconcile requests for the containing namespace

## Understanding Score

0.85

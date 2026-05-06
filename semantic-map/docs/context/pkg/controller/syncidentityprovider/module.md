# Module atlas

## Responsibility

Reconciles SyncIdentityProvider and SelectorSyncIdentityProvider resources into SyncSet objects that patch the OAuth config on remote clusters with aggregated identity provider definitions. Watches all three resource types (ClusterDeployment, SyncIdentityProvider, SelectorSyncIdentityProvider) and creates or updates a single SyncSet per ClusterDeployment containing a merge patch for the cluster OAuth object.

## Public Interface/API

- `const ControllerName` -- hivev1.SyncIdentityProviderControllerName
- `func Add(mgr manager.Manager) error` -- creates and registers the controller
- `func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler` -- returns a new reconciler
- `func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error` -- registers controller with watches on ClusterDeployment, SyncIdentityProvider, SelectorSyncIdentityProvider
- `func GenerateIdentityProviderSyncSetName(clusterDeploymentName string) string` -- generates predictable SyncSet name
- `type ReconcileSyncIdentityProviders struct` -- reconciler; embeds client.Client
- `func (r *ReconcileSyncIdentityProviders) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)` -- main reconcile loop

## Internal Dependencies

- `github.com/openshift/api/config/v1` -- IdentityProvider type
- `github.com/openshift/hive/apis/helpers` -- resource name generation
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet, SyncIdentityProvider, SelectorSyncIdentityProvider
- `github.com/openshift/hive/pkg/constants` -- label keys, suffixes
- `github.com/openshift/hive/pkg/controller/metrics` -- reconcile time observer
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, client wrapper, conditions, ownership, logging
- `github.com/openshift/hive/pkg/util/labels` -- label helpers
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, client, handler, source

## Capabilities

- Watches ClusterDeployments, SyncIdentityProviders, and SelectorSyncIdentityProviders
- Aggregates identity providers from both namespace-scoped SyncIdentityProvider refs and cluster-wide SelectorSyncIdentityProvider label selectors
- Creates a SyncSet with a merge patch targeting the `config.openshift.io/v1 OAuth` object named `cluster`
- Sorts identity providers by name for deterministic patch output
- Sets owner references on derived SyncSets for garbage collection
- Avoids creating a SyncSet with an empty IDP list when no prior SyncSet exists
- Respects reconcile pause annotation

## Understanding Score

0.85

# Module atlas

## Responsibility

Provides a controller that syncs identity provider configuration to remote clusters. When SyncIdentityProvider or SelectorSyncIdentityProvider CRs exist, the controller creates SyncSets containing OAuth configuration that is applied to matching ClusterDeployments' remote clusters to configure authentication providers.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler.
- `ControllerName` -- Constant `"syncidentityprovider"`.
- `GenerateIdentityProviderSyncSetName` -- Generates a predictable SyncSet name for a given CD.
- `ReconcileSyncIdentityProviders` -- Reconciler struct.
- `ReconcileSyncIdentityProviders.Reconcile` -- Creates/updates SyncSets for identity provider configuration.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncIdentityProvider, SelectorSyncIdentityProvider CRs.
- `github.com/openshift/api/config/v1` -- OAuth/IdentityProvider types.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, owner references, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (installed).
- Watches: ClusterDeployment, SyncIdentityProvider, SelectorSyncIdentityProvider.
- Conditions set: None.
- Key logic: Collects identity providers from SyncIdentityProviders (namespace-scoped) and SelectorSyncIdentityProviders (cluster-scoped, matched by label selector). Generates a SyncSet with an OAuth resource containing all matched identity providers. The SyncSet is named `{cd-name}-idp` with owner reference to the CD.

## Understanding Score

0.80

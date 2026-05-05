# Module atlas

## Responsibility

Provides a controller that applies SyncSets and SelectorSyncSets to remote clusters. For each installed ClusterDeployment, the controller identifies applicable SyncSets (by namespace) and SelectorSyncSets (by label selector), then applies their resources and patches to the remote cluster. It tracks application status in a ClusterSync internal API object, handles re-application on a configurable interval (default 2 hours), supports templated resources with Go text/template expansion, and manages the clustersync StatefulSet scaling.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `ControllerName` -- Constant referencing `hivev1.ClustersyncControllerName`.
- `ReconcileClusterSync` -- Reconciler struct.
- `ReconcileClusterSync.Reconcile` -- Applies SyncSets/SelectorSyncSets to remote clusters, tracks status in ClusterSync.
- `CommonSyncSet` -- Interface for treating SyncSet and SelectorSyncSet generically.
- `SyncSetAsCommon` / `SelectorSyncSetAsCommon` -- Adapter types implementing CommonSyncSet.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- SyncSet, SelectorSyncSet CRs.
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- ClusterSync internal API.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client.
- `github.com/openshift/hive/pkg/resource` -- Resource apply/patch operations.
- `github.com/openshift/hive/pkg/controller/utils` -- Conditions, controller config, StatefulSet utilities.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (installed only, based on StatefulSet ordinal assignment).
- Watches: ClusterDeployment, SyncSet, SelectorSyncSet, ClusterSync.
- Conditions set: Updates `SyncSetFailedCondition` on ClusterDeployment. Updates ClusterSync status with per-syncset application results.
- Key logic: Applies resources via apply/createOrUpdate/createOnly modes, applies patches (merge, strategic merge, JSON), supports Go template expansion for resources, tracks first-success-time metrics, reapplies on configurable interval with jitter. Runs as StatefulSet replicas with ordinal-based cluster assignment.
- Metrics: `hive_syncset_apply_duration_seconds`, `hive_selectorsyncset_apply_duration_seconds`, `hive_syncsetinstance_resources_applied_total`, `hive_clustersync_first_success_duration_seconds`.

## Understanding Score

0.80

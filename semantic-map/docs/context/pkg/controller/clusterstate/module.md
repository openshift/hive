# Module atlas

## Responsibility

Provides a controller that maintains ClusterState objects reflecting the status of cluster operators on remote clusters. For each installed ClusterDeployment, the controller periodically (every 10 minutes) connects to the remote cluster, reads the `ClusterVersion` and `ClusterOperator` resources, and updates the corresponding ClusterState object with the current operator status.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler with remote client builder.
- `ControllerName` -- Constant referencing `hivev1.ClusterStateControllerName`.
- `ReconcileClusterState` -- Reconciler struct.
- `ReconcileClusterState.Reconcile` -- Ensures a ClusterState exists for installed CDs and updates it with remote cluster operator status.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterState CRs.
- `github.com/openshift/api/config/v1` -- ClusterOperator types for remote cluster querying.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names, label keys.
- `github.com/openshift/hive/pkg/remoteclient` -- Builds clients for the remote cluster.
- `github.com/openshift/hive/pkg/controller/utils` -- Owner references, conditions, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/openshift/hive/pkg/util/labels` -- Label management utilities.

## Capabilities

- Reconciles: **ClusterDeployment** (installed only).
- Watches: ClusterDeployment.
- Conditions set: None directly (updates ClusterState status).
- Key logic: Creates ClusterState with owner reference to CD, queries remote cluster for ClusterOperator list, updates ClusterState.status.clusterOperators. Requeues after 10 minutes for periodic updates. Skips if cluster is not installed or is paused/relocating.

## Understanding Score

0.85

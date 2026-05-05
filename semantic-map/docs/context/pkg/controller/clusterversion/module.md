# Module atlas

## Responsibility

Provides a controller that syncs the remote cluster's ClusterVersion status to the ClusterDeployment status. For each installed ClusterDeployment, it periodically connects to the remote cluster, reads the `version` ClusterVersion object, and updates `cd.Status.ClusterVersionStatus` with the remote cluster's version information including available updates and version history.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager. Configurable poll interval via env var.
- `AddToManager` -- Adds the controller to a manager.
- `ControllerName` -- Constant referencing `hivev1.ClusterVersionControllerName`.
- `ReconcileClusterVersion` -- Reconciler struct with configurable poll interval.
- `ReconcileClusterVersion.Reconcile` -- Reads remote ClusterVersion and syncs to CD status.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CR.
- `github.com/openshift/api/config/v1` -- ClusterVersion type for remote querying.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names.
- `github.com/openshift/hive/pkg/remoteclient` -- Remote cluster client.
- `github.com/openshift/hive/pkg/controller/utils` -- Controller config, log helpers.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.
- `github.com/blang/semver/v4` -- Version comparison.

## Capabilities

- Reconciles: **ClusterDeployment** (installed only).
- Watches: ClusterDeployment.
- Conditions set: None directly (updates CD status.clusterVersionStatus).
- Key logic: Reads `version` ClusterVersion object from remote cluster, compares with current CD status using go-cmp, updates if changed. Requeues after configurable poll interval (default from env var, with random jitter). Skips unreachable, hibernating, or paused clusters.

## Understanding Score

0.85

# Module atlas

## Responsibility

Provides a controller that manages ClusterProvision objects, which represent individual cluster installation attempts. It creates and monitors install jobs, tracks job progress via pod status and install log monitoring, handles job failures and retries, and updates provision status. The controller also monitors install logs for known failure patterns via regex matching and sets appropriate failure reasons.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager. Reads and registers metrics config.
- `ControllerName` -- Constant referencing `hivev1.ClusterProvisionControllerName`.
- `ReconcileClusterProvision` -- Reconciler struct with expectations for job creation tracking and shared pod config.
- `ReconcileClusterProvision.Reconcile` -- Main reconcile loop. Creates install jobs, monitors progress, handles completion/failure.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterProvision, ClusterDeployment CRs.
- `github.com/openshift/hive/pkg/install` -- Install job generation.
- `github.com/openshift/hive/pkg/controller/utils` -- Expectations, job utilities, conditions, controller config, shared pod config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation, metrics config.
- `github.com/openshift/installer/pkg/types` -- Installer types for log monitoring.

## Capabilities

- Reconciles: **ClusterProvision**.
- Watches: ClusterProvision, Jobs (owner with expectations), ClusterDeployment (mapped).
- Conditions set: Updates ClusterProvision status (stage, conditions on parent CD).
- Key logic: Creates install jobs via `install.GenerateInstallerJob`, monitors job pods for status updates, parses install logs for known failure patterns (via installlogmonitor/installlogregex), updates provision status with failure reasons. Uses expectations to track job creation. Tracks install duration metrics per platform.
- Metrics: Provision duration histograms per platform and result (success/failure).

## Understanding Score

0.80

# Module atlas

## Responsibility

Provides a controller that creates Velero Backup objects when changes are made to Hive resources (ClusterDeployments, SyncSets, DNSZones). The controller is only active when the `VELERO_BACKUP` environment variable is set to `"true"`. It rate-limits backup creation (default 3 minutes between backups) and creates namespace-scoped Velero backups that include all resources in a ClusterDeployment's namespace, excluding pods, jobs, and checkpoints.

## Public Interface/API

- `Add` -- Creates the controller (only if VELERO_BACKUP env var is true).
- `AddToManager` -- Adds the controller to a manager.
- `NewReconciler` -- Returns a new reconciler with configurable rate limiting.
- `ControllerName` -- Constant referencing `hivev1.VeleroBackupControllerName`.
- `ReconcileBackup` -- Reconciler struct.
- `ReconcileBackup.Reconcile` -- Creates Velero Backup objects for changed Hive namespaces.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet, DNSZone CRs.
- `github.com/heptio/velero/pkg/apis/velero/v1` -- Velero Backup CR.
- `github.com/openshift/hive/pkg/constants` -- Environment variable names.
- `github.com/openshift/hive/pkg/controller/utils` -- Checksum calculation, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **Namespace** (triggered by changes to Hive resources within it).
- Watches: ClusterDeployment, SyncSet, DNSZone (all mapped to their namespace).
- Conditions set: None.
- Key logic: Computes checksums of all Hive resources in a namespace. If checksum differs from last backup, creates a new Velero Backup object (sorted, deterministic names). Rate-limits backup creation using configurable minimum period (default 3 minutes, configurable via `MINIMUM_BACKUP_PERIOD_SECONDS` env var). Excludes pods, jobs, and checkpoints from backups.

## Understanding Score

0.85

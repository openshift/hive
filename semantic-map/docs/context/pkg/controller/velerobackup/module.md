# Module atlas

## Responsibility

Creates Velero Backup objects when Hive namespace-scoped resources (ClusterDeployments, SyncSets, DNSZones) change. Tracks per-namespace checksums in Checkpoint objects to detect changes and rate-limits backup creation to avoid excessive backups. Only enabled when the `VELERO_BACKUP` environment variable is set to "true".

## Public Interface/API

- `const ControllerName` -- hivev1.VeleroBackupControllerName
- `func Add(mgr manager.Manager) error` -- creates and registers the controller (no-op unless VELERO_BACKUP=true)
- `func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (reconcile.Reconciler, error)` -- returns a new reconciler with configurable rate limit duration and Velero namespace
- `func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.TypedRateLimiter[reconcile.Request]) error` -- registers controller with watches on ClusterDeployment, SyncSet, DNSZone
- `type ReconcileBackup struct` -- reconciler; embeds client.Client
- `func (r *ReconcileBackup) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error)` -- compares checksums and creates backups if changed

## Internal Dependencies

- `github.com/heptio/velero/pkg/apis/velero/v1` -- Velero Backup type and BackupPhase constants
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, SyncSet, DNSZone, Checkpoint, BackupReference
- `github.com/openshift/hive/pkg/constants` -- env vars (VELERO_BACKUP, MIN_BACKUP_PERIOD_SECONDS, VELERO_NAMESPACE), CheckpointName
- `github.com/openshift/hive/pkg/controller/metrics` -- reconcile time observer
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, client wrapper, checksum, logging, ListRuntimeObjects
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, client, handler, source

## Capabilities

- Conditionally enabled via environment variable
- Watches ClusterDeployment, SyncSet, and DNSZone resources, mapping changes to namespace-level reconcile requests
- Computes MD5 checksums of Hive object specs (excluding status and ResourceVersion) to detect changes
- Rate-limits backup creation per namespace using Checkpoint objects (default 3 minutes, configurable)
- Creates Velero Backup objects in the configurable Velero namespace
- Tracks last backup checksum, time, and reference in per-namespace Checkpoint objects
- Excludes pods, jobs, and checkpoints from backups

## Understanding Score

0.85

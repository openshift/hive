# Module: pkg/operator/metrics

## Responsibility

Provides Prometheus metrics for the Hive operator, including a periodic metrics calculator for HiveConfig conditions and a reusable `ReconcileObserver` for tracking controller reconcile time and outcome across all Hive controllers. This runs as a goroutine added to the operator Manager, not as a standard controller.

## Public Interface/API

- `Add(mgr manager.Manager) error` -- creates a `Calculator` with a 2-minute interval and adds it to the Manager as a runnable.
- `ControllerName` -- constant `hivev1.MetricsControllerName`.
- `Calculator` -- struct with a controller-runtime `Client` and `Interval`:
  - `Start(ctx context.Context) error` -- runs a loop that periodically reads HiveConfig and publishes `hive_hiveconfig_conditions` gauge metrics (per condition type and reason).
- `ReconcileObserver` -- struct for tracking per-reconcile timing and outcome:
  - `NewReconcileObserver(controllerName, logger) *ReconcileObserver` -- captures start time.
  - `ObserveControllerReconcileTime()` -- records elapsed time to `hive_operator_reconcile_seconds` histogram (labeled by controller and outcome), logs "reconcile complete" with elapsed millis and duration bucket.
- `ReconcileOutcome` -- string type for categorizing reconcile results.
  - Constants: `ReconcileOutcomeUnspecified`, `ReconcileOutcomeNoOp`, `ReconcileOutcomeSkippedSync`, `ReconcileOutcomeFullSync`, `ReconcileOutcomeClusterSyncCreated`.

## Prometheus Metrics Registered

1. `hive_hiveconfig_conditions` (GaugeVec) -- labels: `condition`, `reason`. Set to 1 for True conditions, 0 otherwise. Partial-match deleted before refresh to avoid stale values.
2. `hive_operator_reconcile_seconds` (HistogramVec) -- labels: `controller`, `outcome`. Buckets: 0.001, 0.01, 0.1, 1, 10, 30, 60, 120 seconds.

## Internal Dependencies

- `apis/hive/v1` -- HiveConfig type, ControllerName type, HiveReadyCondition
- `pkg/constants` -- `HiveConfigName`
- `sigs.k8s.io/controller-runtime/pkg/metrics` -- `Registry` for Prometheus metric registration
- `github.com/prometheus/client_golang/prometheus` -- gauge/histogram definitions

## Capabilities

- **Package**: `metrics`
- Single source file: `metrics.go` (166 lines).
- 12 unique import paths.
- Used by `pkg/operator/hive` (ReconcileObserver in Reconcile loop) and registered via `pkg/operator` init.

## Understanding Score

0.90

# Module atlas

## Responsibility

Provides Prometheus metrics for the Hive operator: periodically calculates HiveConfig condition gauges, and offers a `ReconcileObserver` for tracking controller reconcile duration and outcome across all Hive controllers.

## Public Interface/API

- `Add(mgr manager.Manager) error` -- creates and registers a metrics Calculator with the manager
- `Calculator` struct -- periodic metrics loop; fields: `Client client.Client`, `Interval time.Duration`
  - `Start(ctx context.Context) error` -- begins the periodic metrics calculation
- `ControllerName` const -- `hivev1.MetricsControllerName`
- `ReconcileObserver` struct -- tracks reconcile start time, controller name, and outcome
  - `NewReconcileObserver(controllerName, logger) *ReconcileObserver`
  - `ObserveControllerReconcileTime()` -- records duration histogram and logs reconcile completion
- `ReconcileOutcome` type with constants: `ReconcileOutcomeUnspecified`, `ReconcileOutcomeNoOp`, `ReconcileOutcomeSkippedSync`, `ReconcileOutcomeFullSync`, `ReconcileOutcomeClusterSyncCreated`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/prometheus/client_golang/prometheus`
- `sigs.k8s.io/controller-runtime/pkg/client`, `manager`, `metrics`

## Capabilities

- Registers `hive_hiveconfig_conditions` gauge vec (condition, reason labels) and `hive_operator_reconcile_seconds` histogram vec (controller, outcome labels)
- Periodically reads HiveConfig and publishes condition status as Prometheus metrics
- Provides reusable reconcile timing and outcome observation for all Hive controllers
- Logs reconcile duration with elapsed-duration bucket classification

## Understanding Score

0.9

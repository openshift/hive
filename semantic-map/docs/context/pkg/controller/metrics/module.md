# Module atlas

## Responsibility

Provides a metrics calculation system for Hive. Includes a `Calculator` that runs as a goroutine within the manager, periodically computing aggregate Prometheus metrics across all ClusterDeployments, ClusterPools, and related objects. Also provides shared metric types (`CounterVecWithDynamicLabels`, `HistogramVecWithDynamicLabels`) supporting optional labels from HiveConfig, `ReconcileObserver` for per-controller reconcile time tracking, and utilities for reading metrics configuration.

## Public Interface/API

- `Add` -- Creates the Calculator and adds it to the Manager.
- `Calculator` -- Goroutine-based metrics calculator (not a standard controller). Periodically computes aggregate metrics.
- `Calculator.Start` -- Begins the periodic metrics calculation loop.
- `ControllerName` -- Constant `"metrics"`.
- `ReconcileObserver` -- Per-reconcile helper that tracks time and outcome. Used by all controllers via `NewReconcileObserver`.
- `ReconcileOutcome` -- Enum for reconcile outcomes (success, error, etc.).
- `ReadMetricsConfig` -- Reads metrics configuration from file (pointed to by env var).
- `GetOptionalClusterTypeLabels` -- Returns additional label names from metrics config.
- `CounterVecWithDynamicLabels` / `HistogramVecWithDynamicLabels` -- Metric wrappers supporting dynamic labels.
- `MetricClusterHibernationTransitionSeconds`, `MetricClusterReadyTransitionSeconds`, `MetricResumingClustersSeconds`, `MetricStoppingClustersSeconds`, `MetricWaitingForCOClustersSeconds` -- Shared metric variables used by other controllers.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterPool, ClusterProvision CRs.
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1` -- ClusterSync.
- `github.com/openshift/hive/apis/hive/v1/metricsconfig` -- Metrics configuration types.
- `github.com/openshift/hive/pkg/constants` -- Env var names for metrics config.
- `github.com/openshift/hive/pkg/controller/utils` -- Condition helpers, controller config.
- `github.com/openshift/hive/pkg/imageset` -- Image set utilities.

## Capabilities

- Not a standard controller -- runs as a goroutine via `Calculator.Start`.
- Periodically calculates aggregate metrics: cluster counts by state/platform/type, provision metrics, pool metrics, condition-based metrics.
- Provides per-controller `ReconcileObserver` for reconcile timing and outcome tracking.
- Supports dynamic labels from HiveConfig `metricsConfig.additionalClusterDeploymentLabels`.

## Understanding Score

0.80

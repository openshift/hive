# Module atlas

## Responsibility

Provides the central Prometheus metrics infrastructure for Hive controllers. Runs a periodic Calculator goroutine that computes aggregate ClusterDeployment, job, and SyncSet metrics, and exposes the ReconcileObserver utility used by all controllers to track reconcile loop duration and outcomes. Also supports dynamic metric labels configurable via HiveConfig.

## Public Interface/API

- `ControllerName` -- constant `hivev1.MetricsControllerName`
- `Add(mgr manager.Manager) error` -- creates and registers Calculator, custom collectors
- `Calculator` -- goroutine that periodically computes aggregate metrics
- `Calculator.Start(ctx context.Context) error` -- begins the metrics calculation loop
- `ReconcileObserver` -- tracks per-reconcile timing and outcome
- `NewReconcileObserver(controllerName, logger) *ReconcileObserver`
- `ReconcileObserver.ObserveControllerReconcileTime()` -- records reconcile duration metric
- `ReconcileObserver.SetOutcome(outcome ReconcileOutcome)`
- `ReconcileOutcome` -- type with constants: `Unspecified`, `NoOp`, `SkippedSync`, `FullSync`, `ClusterSyncCreated`
- `CounterVecWithDynamicLabels` -- counter metric with optional labels from HiveConfig
- `HistogramVecWithDynamicLabels` -- histogram metric with optional labels from HiveConfig
- `GetLabelValue(obj, label) string` -- returns label value or default
- `GetPowerStateValue(powerState) string` -- returns power state string or default
- `GetOptionalClusterTypeLabels(mConfig) map[string]string` -- reads AdditionalClusterDeploymentLabels
- `ReadMetricsConfig() (*metricsconfig.MetricsConfig, error)` -- reads config from env-var-pointed file
- `ShouldLogHistogramDurationMetric(metric, timeToLog) bool` -- checks if optional histogram metric should be logged
- Exported histogram vars: `MetricClusterHibernationTransitionSeconds`, `MetricClusterReadyTransitionSeconds`, `MetricStoppingClustersSeconds`, `MetricResumingClustersSeconds`, `MetricWaitingForCOClustersSeconds`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `apis/hive/v1/metricsconfig`, `apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/utils` -- conditions, label helpers
- `github.com/openshift/hive/pkg/imageset` -- imageset job label
- `github.com/prometheus/client_golang/prometheus` -- metric types
- `sigs.k8s.io/controller-runtime/pkg/metrics` -- registry

## Capabilities

- Periodically lists all ClusterDeployments and computes aggregate gauges: total, installed, uninstalled, deprovisioning, by conditions, by age buckets, by cluster type
- Tracks install/uninstall/imageset job counts by state (running, succeeded, failed)
- Tracks SelectorSyncSet and SyncSet applied/unapplied counts
- Custom collectors for provisioning-underway and deprovisioning-underway durations
- Provides per-controller reconcile time histogram with outcome labels
- Supports dynamic optional labels on metrics configured via HiveConfig.Spec.MetricsConfig
- Registers optional duration-based metrics (stopping, resuming, waiting-for-CO, hibernation/resume transitions)

## Understanding Score

0.85

<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/metrics/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new metrics Calculator and adds it to the Manager.
- `Calculator` — Calculator runs in a goroutine and periodically calculates and publishes Prometheus metrics which will be exposed at our /metrics endpoint. Note that this is not a standard contro…
- `Calculator.Start` — Start begins the metrics calculation loop.
- `ControllerName`
- `CounterVecWithDynamicLabels`
- `CounterVecWithDynamicLabels.Observe` — Observe logs the counter metric. It simply increments the value, so last parameter isn't used.
- `CounterVecWithDynamicLabels.Register` — Register registers the CounterVec metric.
- `GetLabelValue` — GetLabelValue returns the value of the label if set, otherwise a default value.
- `GetOptionalClusterTypeLabels` — GetOptionalClusterTypeLabels reads the AdditionalClusterDeploymentLabels from the metrics config and returns the same as a map
- `GetPowerStateValue` — GetPowerStateValue returns the value of the power state if set, otherwise a default value.
- `HistogramVecWithDynamicLabels`
- `HistogramVecWithDynamicLabels.Observe` — Observe observes the histogram metric with val value.
- `HistogramVecWithDynamicLabels.Register` — Register registers the HistogramVec metric.
- `MetricClusterHibernationTransitionSeconds`
- `MetricClusterReadyTransitionSeconds`
- `MetricResumingClustersSeconds`
- `MetricStoppingClustersSeconds`
- `MetricWaitingForCOClustersSeconds`
- `ReadMetricsConfig` — ReadMetricsConfig reads the metrics config from the file pointed to by the MetricsConfigFileEnvVar environment variable.
- `ReconcileObserver` — ReconcileObserver is used to track, log, and report metrics for controller reconcile time and outcome. Each controller should instantiate one near the start of the reconcile loop,…
- `ReconcileObserver.ObserveControllerReconcileTime`
- `ReconcileObserver.SetOutcome`
- `ReconcileOutcome` — ReconcileOutcome is used in controller "reconcile complete" log entries, and the metricControllerReconcileTime above for controllers where we would like to monitor performance for…
- `ShouldLogHistogramDurationMetric` — ShouldLogHistogramDurationMetric decides whether the corresponding duration metric of type histogram should be logged. It first checks if that optional metric has been opted into …

## Internal Dependencies

- `context`
- `encoding/json`
- `errors`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/metricsconfig`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/imageset`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/apimachinery/pkg/util/wait`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `strconv`
- `time`

## Capabilities

- **`package`** name(s): **metrics**.
- Go **`import`** edges listed below (24 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/metrics`.

## Understanding Score

0.0

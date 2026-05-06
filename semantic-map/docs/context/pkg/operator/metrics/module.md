<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/operator/metrics/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new metrics Calculator and adds it to the Manager.
- `Calculator` — Calculator runs in a goroutine and periodically calculates and publishes Prometheus metrics which will be exposed at our /metrics endpoint. Note that this is not a standard contro…
- `Calculator.Start` — Start begins the metrics calculation loop.
- `ControllerName`
- `ReconcileObserver` — ReconcileObserver is used to track, log, and report metrics for controller reconcile time and outcome. Each controller should instantiate one near the start of the reconcile loop,…
- `ReconcileObserver.ObserveControllerReconcileTime`
- `ReconcileOutcome` — ReconcileOutcome is used in controller "reconcile complete" log entries, and the metricControllerReconcileTime above for controllers where we would like to monitor performance for…

## Internal Dependencies

- `context`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/wait`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `time`

## Capabilities

- **`package`** name(s): **metrics**.
- Go **`import`** edges listed below (12 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/operator/metrics`.

## Understanding Score

0.0

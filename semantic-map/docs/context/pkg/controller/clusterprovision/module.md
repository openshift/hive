<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterprovision/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterProvision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `ControllerName`
- `ReconcileClusterProvision` — ReconcileClusterProvision reconciles a ClusterProvision object
- `ReconcileClusterProvision.Reconcile` — Reconcile reads that state of the cluster for a ClusterProvision object and makes changes based on the state read and what is in the ClusterProvision.Spec

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/metricsconfig`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/install`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/openshift/installer/pkg/types`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sigs.k8s.io/yaml`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **clusterprovision**.
- Go **`import`** edges listed below (34 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterprovision`.

## Understanding Score

0.0

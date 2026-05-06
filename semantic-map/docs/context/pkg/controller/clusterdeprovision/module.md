<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterdeprovision/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Actuator` — Actuator interface is the interface that is used to add cloud provider support to the deprovision controller.
- `Add` — Add creates a new ClusterDeprovision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Starte…
- `ControllerName`
- `ReconcileClusterDeprovision` — ReconcileClusterDeprovision reconciles a ClusterDeprovision object
- `ReconcileClusterDeprovision.Reconcile` — Reconcile reads that state of the cluster for a ClusterDeprovision object and makes changes based on the state read and what is in the ClusterDeprovision.Spec

## Internal Dependencies

- `context`
- `fmt`
- `github.com/aws/aws-sdk-go-v2/service/sts`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/install`
- `github.com/openshift/hive/pkg/util/labels`
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
- `os`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `strings`

## Capabilities

- **`package`** name(s): **clusterdeprovision**.
- Go **`import`** edges listed below (31 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterdeprovision`.

## Understanding Score

0.0

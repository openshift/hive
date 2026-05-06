<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterversion/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `ReconcileClusterVersion` — ReconcileClusterVersion reconciles a ClusterDeployment object
- `ReconcileClusterVersion.Reconcile` — Reconcile reads that state of the cluster for a ClusterDeployment object and syncs the remote ClusterVersion status if the remote cluster is available.

## Internal Dependencies

- `cmp`
- `context`
- `fmt`
- `github.com/blang/semver/v4`
- `github.com/google/go-cmp/cmp`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/sirupsen/logrus`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/util/workqueue`
- `math/rand`
- `os`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `time`

## Capabilities

- **`package`** name(s): **clusterversion**.
- Go **`import`** edges listed below (26 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterversion`.

## Understanding Score

0.0

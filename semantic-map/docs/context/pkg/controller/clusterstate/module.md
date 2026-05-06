<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterstate/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterState controller and adds it to the manager with default RBAC.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileClusterState` — ReconcileClusterState is the reconciler for ClusterState. It will sync on ClusterDeployment resources and ensure that a ClusterState exists and is updated when appropriate.
- `ReconcileClusterState.Reconcile` — Reconcile ensures that a given ClusterState resource exists and reflects the state of cluster operators from its target cluster

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/sirupsen/logrus`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `time`

## Capabilities

- **`package`** name(s): **clusterstate**.
- Go **`import`** edges listed below (26 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterstate`.

## Understanding Score

0.0

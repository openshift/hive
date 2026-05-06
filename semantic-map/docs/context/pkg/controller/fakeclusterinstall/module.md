<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/fakeclusterinstall/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new FakeClusterInstall controller and adds it to the manager with default RBAC.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileClusterInstall` — ReconcileClusterInstall is the reconciler for FakeClusterInstall.
- `ReconcileClusterInstall.Reconcile` — Reconcile ensures that a given FakeClusterInstall resource exists and reflects the state of cluster operators from its target cluster

## Internal Dependencies

- `context`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/library-go/pkg/controller`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/utils/ptr`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `time`

## Capabilities

- **`package`** name(s): **fakeclusterinstall**.
- Go **`import`** edges listed below (21 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/fakeclusterinstall`.

## Understanding Score

0.0

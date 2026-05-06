<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/unreachable/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new Unreachable Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileRemoteMachineSet` — ReconcileRemoteMachineSet reconciles the MachineSets generated from a ClusterDeployment object
- `ReconcileRemoteMachineSet.Reconcile` — Reconcile checks if we can establish an API client connection to the remote cluster and maintains the unreachable condition as a result.

## Internal Dependencies

- `context`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `time`

## Capabilities

- **`package`** name(s): **unreachable**.
- Go **`import`** edges listed below (21 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/unreachable`.

## Understanding Score

0.0

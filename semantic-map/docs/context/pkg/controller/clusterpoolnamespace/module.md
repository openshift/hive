<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterpoolnamespace/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileClusterPoolNamespace` — ReconcileClusterPoolNamespace reconciles a Namespace object for the purpose of reaping namespaces created for ClusterPool clusters after the clusters have been deleted.
- `ReconcileClusterPoolNamespace.Reconcile` — Reconcile deletes a Namespace if it no longer contains any ClusterDeployments.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `time`

## Capabilities

- **`package`** name(s): **clusterpoolnamespace**.
- Go **`import`** edges listed below (19 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterpoolnamespace`.

## Understanding Score

0.0

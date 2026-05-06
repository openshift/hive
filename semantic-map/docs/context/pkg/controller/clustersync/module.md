<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clustersync/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new clustersync Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `CommonSyncSet` — CommonSyncSet is an interface for interacting with SyncSets and SelectorSyncSets in a generic way.
- `ControllerName`
- `ReconcileClusterSync` — ReconcileClusterSync reconciles a ClusterDeployment object to apply its SyncSets and SelectorSyncSets
- `ReconcileClusterSync.Reconcile` — Reconcile reads the state of the ClusterDeployment and applies any SyncSets or SelectorSyncSets that need to be applied or re-applied.
- `SelectorSyncSetAsCommon` — SelectorSyncSetAsCommon is a SelectorSyncSet typed as a CommonSyncSet
- `SelectorSyncSetAsCommon.AsMetaObject`
- `SelectorSyncSetAsCommon.AsRuntimeObject`
- `SelectorSyncSetAsCommon.GetSpec`
- `SyncSetAsCommon` — SyncSetAsCommon is a SyncSet typed as a CommonSyncSet
- `SyncSetAsCommon.AsMetaObject`
- `SyncSetAsCommon.AsRuntimeObject`
- `SyncSetAsCommon.GetSpec`

## Internal Dependencies

- `bytes`
- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/resource`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/labels`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/json`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `math/rand`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sigs.k8s.io/yaml`
- `sort`
- `strings`
- `text/template`
- `time`

## Capabilities

- **`package`** name(s): **clustersync**.
- Go **`import`** edges listed below (39 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clustersync`.

## Understanding Score

0.0

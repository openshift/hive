<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/controlplanecerts/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ControlPlaneCerts Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ControllerName`
- `GenerateControlPlaneCertsSyncSetName` — GenerateControlPlaneCertsSyncSetName generates the name of the SyncSet that holds the control plane certificates to sync.
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileControlPlaneCerts` — ReconcileControlPlaneCerts reconciles a ClusterDeployment object
- `ReconcileControlPlaneCerts.Reconcile` — Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read and what is in the ClusterDeployment.Spec

## Internal Dependencies

- `context`
- `crypto/md5`
- `fmt`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `io`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `net/url`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sort`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **controlplanecerts**.
- Go **`import`** edges listed below (34 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/controlplanecerts`.

## Understanding Score

0.0

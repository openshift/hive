<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterrelocate/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterRelocate controller and adds it to the manager with default RBAC.
- `ControllerName`
- `ReconcileClusterRelocate` — ReconcileClusterRelocate is the reconciler for ClusterRelocate. It will sync on ClusterDeployment resources and relocate those that match with a ClusterRelocate.
- `ReconcileClusterRelocate.Reconcile` — Reconcile relocates ClusterDeployments matching with a ClusterRelocate to another Hive instance.

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/labels`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/diff`
- `k8s.io/apimachinery/pkg/util/sets`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `strings`

## Capabilities

- **`package`** name(s): **clusterrelocate**.
- Go **`import`** edges listed below (30 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterrelocate`.

## Understanding Score

0.0

<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/argocdregister/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new Argocdregister Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ArgoCDRegisterController` — ArgoCDRegisterController reconciles ClusterDeployments and generates ArgoCD cluster secrets for ClusterDeployments
- `ArgoCDRegisterController.Reconcile` — Reconcile checks if we can establish an API client connection to the remote cluster and maintains the unreachable condition as a result.
- `ClusterConfig` — ClusterConfig is the configuration attributes. This structure is subset of the go-client rest.Config with annotations added for marshalling.
- `ControllerName`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `TLSClientConfig` — TLSClientConfig contains settings to enable transport layer security

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/sirupsen/logrus`
- `hash/fnv`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/clientcmd`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `net/url`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `strconv`
- `strings`

## Capabilities

- **`package`** name(s): **argocdregister**.
- Go **`import`** edges listed below (30 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/argocdregister`.

## Understanding Score

0.0

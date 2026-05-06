<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/resource/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ApplyResult` — ApplyResult indicates what type of change was performed by calling the Apply function
- `DeleteAnyExistingObject` — DeleteAnyExistingObject will look for any object that exists that matches the passed in 'obj' and will delete it if it exists. The first return value is true iff the object was al…
- `GenerateClientConfigFromRESTConfig` — GenerateClientConfigFromRESTConfig generates a new kubeconfig using a given rest.Config. The rest.Config may come from in-cluster config (as in a pod) or an existing kubeconfig.
- `Helper`
- `HelperOpt`
- `Info` — Info contains information obtained from a resource submitted to the Apply function
- `Serialize` — Serialize uses a custom JSON extension to properly determine whether metav1.Time should be serialized or not. In cases where a metav1.Time is labeled as 'omitempty', the default j…

## Internal Dependencies

- `bytes`
- `context`
- `fmt`
- `github.com/jonboulle/clockwork`
- `github.com/json-iterator/go`
- `github.com/modern-go/reflect2`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `io`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/cli-runtime/pkg/genericclioptions`
- `k8s.io/cli-runtime/pkg/printers`
- `k8s.io/cli-runtime/pkg/resource`
- `k8s.io/client-go/discovery`
- `k8s.io/client-go/discovery/cached/disk`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/restmapper`
- `k8s.io/client-go/tools/clientcmd`
- `k8s.io/client-go/tools/clientcmd/api`
- `k8s.io/kubectl/pkg/cmd/apply`
- `k8s.io/kubectl/pkg/cmd/delete`
- `k8s.io/kubectl/pkg/cmd/patch`
- `k8s.io/kubectl/pkg/cmd/util`
- `k8s.io/kubectl/pkg/util/openapi`
- `math/rand`
- `os`
- `path/filepath`
- `reflect`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `strings`
- `time`
- `unsafe`

## Capabilities

- **`package`** name(s): **resource**.
- Go **`import`** edges listed below (42 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/resource`.

## Understanding Score

0.0

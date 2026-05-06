# Module atlas

## Responsibility

Provides a kubectl-based resource helper abstraction for applying, patching, deleting, and inspecting Kubernetes resources from serialized YAML/JSON or runtime objects. Wraps kubectl apply/patch/delete commands programmatically with factory-based configuration supporting both in-cluster and remote kubeconfig targets.

## Public Interface/API

- `Helper` interface -- `Apply(obj []byte) (ApplyResult, error)`, `ApplyRuntimeObject(obj, scheme)`, `CreateOrUpdate(obj []byte)`, `CreateOrUpdateRuntimeObject(obj, scheme)`, `Create(obj []byte)`, `CreateRuntimeObject(obj, scheme)`, `Info(obj []byte) (*Info, error)`, `Patch(name, kind, apiVersion, patch, patchType)`, `Delete(apiVersion, kind, namespace, name)`
- `NewHelper(logger, opts ...HelperOpt) (Helper, error)` -- creates a helper with OpenAPI schema caching
- `NewFakeHelper(logger) Helper` -- creates a no-op helper for scale testing simulations
- `HelperOpt` type -- functional options: `FromRESTConfig(restConfig)`, `FromKubeconfig(kubeconfig []byte)`, `WithMetrics()`, `WithControllerName(cn)`
- `ApplyResult` type -- constants: `ConfiguredApplyResult`, `UnchangedApplyResult`, `CreatedApplyResult`, `UnknownApplyResult`
- `Info` struct -- Name, Namespace, APIVersion, Kind, Resource, Object
- `DeleteAnyExistingObject(c client.Client, key, obj, logger) (bool, error)` -- deletes existing object if found
- `GenerateClientConfigFromRESTConfig(name string, restConfig *rest.Config) *configapi.Config` -- generates kubeconfig from REST config
- `Serialize(obj runtime.Object, scheme) ([]byte, error)` -- custom JSON serialization handling metav1.Time omitempty

## Internal Dependencies

- `k8s.io/kubectl/pkg/cmd/apply`, `delete`, `patch`, `util`
- `k8s.io/cli-runtime/pkg/genericclioptions`, `resource`, `printers`
- `k8s.io/client-go/rest`, `discovery`, `restmapper`, `tools/clientcmd`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `github.com/json-iterator/go`, `github.com/modern-go/reflect2`
- `github.com/jonboulle/clockwork`

## Capabilities

- Programmatic kubectl apply with server-side change tracking (configured/unchanged/created)
- Programmatic kubectl patch supporting JSON, merge, and strategic merge patch types
- Programmatic kubectl delete via dynamic client
- Resource info extraction (name, namespace, GVK) from raw bytes
- Kubeconfig generation from REST config for remote cluster operations
- Custom JSON serialization for metav1.Time with proper omitempty handling
- OpenAPI schema caching for performance across multiple apply calls
- Fake helper with simulated latency for scale testing
- Disk-cached discovery for API group resolution

## Understanding Score

0.9

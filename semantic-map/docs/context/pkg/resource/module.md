# Module: pkg/resource

## Responsibility

Wraps kubectl-style operations (apply, create, create-or-update, patch, delete, info) behind a `Helper` interface, executing them programmatically against local or remote clusters using raw byte manifests or `runtime.Object`. The package constructs `cmdutil.Factory` instances from either a kubeconfig byte slice or a `rest.Config`, manages disk-cached discovery clients, and provides a custom JSON serializer that correctly handles `omitempty` on `metav1.Time` fields to prevent unnecessary patches. A `fakeHelper` implementation supports scale testing with simulated latency. Also provides `DeleteAnyExistingObject` as a standalone utility and `GenerateClientConfigFromRESTConfig` for kubeconfig generation.

## Public Interface/API

- **`Helper`** (interface) -- Main abstraction. Methods:
  - `Apply(obj []byte) (ApplyResult, error)` -- Server-side-style apply via kubectl apply internals.
  - `ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)` -- Serializes then applies.
  - `CreateOrUpdate(obj []byte) (ApplyResult, error)` -- Creates if absent, patches if present.
  - `CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)` -- Serializes then create-or-update.
  - `Create(obj []byte) (ApplyResult, error)` -- Creates only if not already present; no-op if exists.
  - `CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (ApplyResult, error)` -- Serializes then create.
  - `Info(obj []byte) (*Info, error)` -- Parses resource bytes to extract name, namespace, kind, apiVersion, resource type.
  - `Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error` -- Applies a JSON/merge/strategic patch.
  - `Delete(apiVersion, kind, namespace, name string) error` -- Deletes a resource by GVK and name.
- **`NewHelper(logger, ...HelperOpt) (Helper, error)`** -- Constructor; caches OpenAPI schema on init.
- **`HelperOpt`** (functional option type):
  - `FromRESTConfig(restConfig *rest.Config)` -- Use a REST config for the factory.
  - `FromKubeconfig(kubeconfig []byte)` -- Use raw kubeconfig bytes for the factory.
  - `WithMetrics()` -- Enable metrics transport wrapper.
  - `WithControllerName(cn hivev1.ControllerName)` -- Set field manager prefix.
- **`ApplyResult`** (string type) -- Enum: `ConfiguredApplyResult`, `UnchangedApplyResult`, `CreatedApplyResult`, `UnknownApplyResult`.
- **`Info`** (struct) -- Fields: `Name`, `Namespace`, `APIVersion`, `Kind`, `Resource`, `Object (*unstructured.Unstructured)`.
- **`Serialize(obj runtime.Object, scheme *runtime.Scheme) ([]byte, error)`** -- Custom JSON serialization that correctly omits zero `metav1.Time` values tagged `omitempty`.
- **`DeleteAnyExistingObject(c client.Client, key client.ObjectKey, obj hivev1.MetaRuntimeObject, logger) (bool, error)`** -- Standalone helper that gets and deletes an object via controller-runtime client; returns true if already gone.
- **`GenerateClientConfigFromRESTConfig(name string, restConfig *rest.Config) *configapi.Config`** -- Builds a clientcmd Config from a rest.Config, reading service account token if needed.
- **`NewFakeHelper(logger) Helper`** -- Returns a no-op Helper that simulates latency for scale testing.

### Unexported but notable

- `helper` struct -- Real implementation; holds logger, cache dir, controller name, kubeconfig/restConfig, factory getter, cached OpenAPI schema.
- `kubeconfigClientGetter` / `restConfigClientGetter` -- Implement `genericclioptions.RESTClientGetter` to bridge kubeconfig/rest.Config into `cmdutil.Factory`.
- `getDiscoveryClient(config, cacheDir)` -- Creates a disk-cached discovery client.
- `changeTracker` / `trackerPrinter` -- Intercept kubectl apply's printer to determine the apply result (created/configured/unchanged).
- `setPrivateFieldManagerHACK` -- Uses reflection to set the private `fieldManager` on `PatchOptions` (workaround for upstream API).
- `metaTimeExtension` -- json-iterator extension that makes `metav1.Time` honor `omitempty`.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- `ControllerName`, `MetaRuntimeObject`
- `github.com/openshift/hive/pkg/controller/utils` -- `AddControllerMetricsTransportWrapper`
- `k8s.io/kubectl/pkg/cmd/{apply,delete,patch,util}` -- Programmatic kubectl commands
- `k8s.io/cli-runtime/pkg/{genericclioptions,printers,resource}` -- CLI runtime support
- `k8s.io/client-go/{discovery,discovery/cached/disk,rest,restmapper,tools/clientcmd}` -- Client libraries
- `k8s.io/apimachinery/pkg/{api/errors,api/meta,apis/meta/v1,runtime,types,util/sets}` -- API machinery
- `github.com/json-iterator/go`, `github.com/modern-go/reflect2` -- Custom JSON serialization
- `github.com/jonboulle/clockwork` -- Clock abstraction for patcher backoff

## Capabilities

- **Package name**: `resource`
- **Package ID**: `github.com/openshift/hive/pkg/resource`
- **Source files**: `helper.go`, `apply.go`, `delete.go`, `patch.go`, `info.go`, `client.go`, `serializer.go`, `fake.go`, `kubeconfig_factory.go`, `restconfig_factory.go`, `factory_discovery.go`
- **Test files**: `patch_test.go`
- 42 unique import paths

## Understanding Score

0.85

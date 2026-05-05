# Module: pkg/remoteclient

## Responsibility

Provides a `Builder` abstraction for constructing Kubernetes API clients (controller-runtime, dynamic, and typed kubeclient) that connect to **remote** managed clusters. The package handles kubeconfig retrieval from Secrets, API URL overrides (primary/secondary), IP-level dial overrides, reachability checking, and setting the `Unreachable` condition on ClusterDeployments. It also supports fake clusters for scale testing, returning pre-populated test clients instead of real connections.

## Public Interface/API

- **`Builder`** (interface) -- Central abstraction for building remote cluster clients. Methods:
  - `Build() (client.Client, error)` -- Returns a static controller-runtime client; verifies reachability via discovery.
  - `BuildDynamic() (dynamic.Interface, error)` -- Returns a dynamic Kubernetes client.
  - `BuildKubeClient() (kubeclient.Interface, error)` -- Returns a typed kubernetes clientset.
  - `RESTConfig() (*rest.Config, error)` -- Returns the REST config for the remote cluster.
  - `UsePrimaryAPIURL() Builder` -- Selects the primary API URL (override if set, else default).
  - `UseSecondaryAPIURL() Builder` -- Selects the secondary API URL (initial URL when override exists).
- **`NewBuilder(c client.Client, cd *hivev1.ClusterDeployment, controllerName hivev1.ControllerName) Builder`** -- Creates a Builder from a ClusterDeployment. Returns a `fakeBuilder` if the CD carries the fake-cluster annotation.
- **`NewBuilderFromKubeconfig(c client.Client, secret *corev1.Secret, controllerName hivev1.ControllerName) Builder`** -- Creates a Builder directly from a kubeconfig Secret (no CD required, no URL override support).
- **`ConnectToRemoteCluster(cd, builder, localClient, logger) (client.Client, unreachable, requeue bool)`** -- Convenience function: checks Unreachable condition, calls `Build()`, and if connection fails, sets `UnreachableCondition` on the CD and updates status.
- **`InitialURL(c client.Client, cd *hivev1.ClusterDeployment) (string, error)`** -- Extracts the initial API server URL from the kubeconfig Secret.
- **`Unreachable(cd) (bool, time.Time)`** -- Checks the CD's conditions to determine if the cluster is currently marked unreachable (does not probe).
- **`IsPrimaryURLActive(cd) bool`** -- Returns true if the primary (possibly overridden) API URL is the active one.
- **`SetUnreachableCondition(cd, error) bool`** -- Sets or clears the `Unreachable` condition on a ClusterDeployment.

### Unexported but notable

- `builder` struct -- Real implementation; reads admin kubeconfig from Secret, applies API URL/IP overrides, adds metrics transport wrapper with controller name.
- `kubeconfigBuilder` struct -- Simpler builder from a raw kubeconfig Secret; no URL override logic.
- `fakeBuilder` struct -- Returns test-fake clients populated with dummy Routes, ClusterVersion, and ~30 ClusterOperators for scale testing.
- `createDialContext(dialer, apiServerIPOverride)` -- Produces a custom `DialContext` function that redirects TCP connections to an overridden IP address (used for `APIServerIPOverride`).

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment CRD types, condition types
- `github.com/openshift/hive/pkg/controller/utils` -- `IsFakeCluster`, `FindCondition`, `SetClusterDeploymentConditionWithChangeCheck`, `RestConfigFromSecret`, `AddControllerMetricsTransportWrapper`, `ErrorScrub`
- `github.com/openshift/hive/pkg/util/scheme` -- `GetScheme()` for controller-runtime client
- `github.com/openshift/hive/pkg/test/fake` -- `NewFakeClientBuilder` (used by fakeBuilder)
- `github.com/openshift/api/config/v1`, `github.com/openshift/api/route/v1` -- Types for fake cluster objects
- `k8s.io/client-go/{discovery,dynamic,kubernetes,rest,restmapper}` -- Kubernetes client libraries
- `sigs.k8s.io/controller-runtime/pkg/client` -- Controller-runtime client interface

## Capabilities

- **Package name**: `remoteclient`
- **Package ID**: `github.com/openshift/hive/pkg/remoteclient`
- **Source files**: `remoteclient.go`, `kubeconfig.go`, `dialer.go`, `fake.go`
- **Test files**: `remoteclient_test.go`, `dialer_test.go`
- 23 unique import paths

## Understanding Score

0.85

# Module atlas

## Responsibility

Provides a `Builder` interface and implementations for constructing API clients (controller-runtime, dynamic, kubernetes) to remote clusters managed by Hive. Handles API URL overrides, IP overrides, reachability checks, and the Unreachable condition on ClusterDeployments.

## Public Interface/API

- `Builder` interface -- `Build() (client.Client, error)`, `BuildDynamic() (dynamic.Interface, error)`, `BuildKubeClient() (kubernetes.Interface, error)`, `RESTConfig() (*rest.Config, error)`, `UsePrimaryAPIURL() Builder`, `UseSecondaryAPIURL() Builder`
- `NewBuilder(c client.Client, cd *hivev1.ClusterDeployment, controllerName) Builder` -- creates a builder from a ClusterDeployment (or fake builder for fake clusters)
- `NewBuilderFromKubeconfig(c client.Client, secret *corev1.Secret, controllerName) Builder` -- creates a builder from a kubeconfig secret directly
- `ConnectToRemoteCluster(cd, remoteClientBuilder, localClient, logger) (client.Client, unreachable, requeue bool)` -- connects to remote cluster, sets unreachable condition on failure
- `InitialURL(c client.Client, cd *hivev1.ClusterDeployment) (string, error)` -- returns the initial API URL
- `Unreachable(cd *hivev1.ClusterDeployment) (unreachable bool, lastCheck time.Time)` -- checks unreachable condition
- `IsPrimaryURLActive(cd *hivev1.ClusterDeployment) bool` -- checks if primary API URL (override) is active
- `SetUnreachableCondition(cd *hivev1.ClusterDeployment, connectionError error) bool` -- sets/clears the Unreachable condition

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/hive/pkg/test/fake`
- `k8s.io/client-go/discovery`, `dynamic`, `kubernetes`, `rest`, `restmapper`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- Builds controller-runtime, dynamic, and kubernetes clients from admin kubeconfig secrets
- Supports API URL override and API server IP override with custom dialer
- Reachability verification via discovery API call before returning client
- Manages Unreachable and ActiveAPIURLOverride conditions on ClusterDeployment
- Fake client builder for scale testing with simulated clusters

## Understanding Score

0.9

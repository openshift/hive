# Module atlas

## Responsibility

Shared utility library for Hive end-to-end tests. Provides helpers to construct various Kubernetes clients, retrieve ClusterDeployment resources, and poll/wait for specific resource states (deployments, API services, machines, machine sets, machine pools, nodes, services). These are designed as "must" (fatal-on-error) or wait-with-timeout patterns used by the e2e test suite.

## Public Interface/API

**Client construction** (`client.go`):
- `MustGetConfig() *rest.Config` -- gets kubeconfig from default config resolution; fatals on error.
- `MustGetClient() client.WithWatch` -- returns a controller-runtime client using the Hive singleton scheme.
- `MustGetClientFromConfig(cfg *rest.Config) client.WithWatch` -- same, from an explicit config.
- `MustGetKubernetesClient() kclient.Interface` -- returns a typed kubernetes client.
- `MustGetAPIRegistrationClient() apiregv1client.ApiregistrationV1Interface` -- returns an API registration client.
- `MustGetDynamicClient() dynamic.Interface` -- returns a dynamic client.

**ClusterDeployment helpers** (`clusterdeployment.go`):
- `MustGetClusterDeployment() *hivev1.ClusterDeployment` -- fetches a CD by `CLUSTER_NAME`/`CLUSTER_NAMESPACE` env vars; fatals if missing.
- `MustGetInstalledClusterDeployment() *hivev1.ClusterDeployment` -- same, but also verifies the CD is installed with valid metadata.
- `MustGetClusterDeploymentClientConfig() *rest.Config` -- builds a REST config for connecting to the remote managed cluster via `remoteclient.NewBuilder`.

**Wait/poll utilities**:
- `WaitForDeploymentReady(c, namespace, name, timeout)` -- waits for a Deployment's Available condition to become True (`deployment.go`).
- `WaitForDeployment(c, namespace, name, testFunc, timeout)` -- generic Deployment watcher with a custom test function (`deployment.go`).
- `WaitForAPIServiceAvailable(c, name, timeout)` -- waits for an APIService's Available condition (`apiservice.go`).
- `WaitForAPIService(c, name, testFunc, timeout)` -- generic APIService watcher (`apiservice.go`).
- `WaitForMachines(cfg, testFunc, timeout)` -- watches Machine objects in `openshift-machine-api` namespace (`machine.go`).
- `WaitForMachineSets(cfg, testFunc, timeout)` -- watches MachineSet objects (`machineset.go`).
- `WaitForNodes(cfg, testFunc, timeout)` -- watches Node objects (`node.go`).
- `WaitForService(c, namespace, name, testFunc, timeout)` -- watches a specific Service (`service.go`).

**Resource helpers**:
- `GetMachinePool(c, cd, poolName) *hivev1.MachinePool` -- fetches a MachinePool by conventional name `${cd.Name}-${poolName}`; returns nil if not found (`machinepool.go`).
- `DynamicWaitForDeletion(dynamicClient, gvr, namespace, name, logger) error` -- polls until a resource no longer exists, with a 30-second timeout (`utils.go`).
- `GetHiveNamespaceOrDie() string` -- reads `HIVE_NAMESPACE` env var or fatals (`utils.go`).
- `GetHiveOperatorNamespaceOrDie() string` -- reads `HIVE_OPERATOR_NAMESPACE` env var or fatals (`utils.go`).

**Diff utility**:
- `JSONDiff(a, b any) ([]byte, error)` -- computes a JSON merge patch between two objects (`diff.go`).

## Internal Dependencies

Hive packages:
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, MachinePool types.
- `github.com/openshift/hive/pkg/constants` -- `HiveNamespaceEnvVar`.
- `github.com/openshift/hive/pkg/operator/hive` -- `HiveOperatorNamespaceEnvVar`.
- `github.com/openshift/hive/pkg/remoteclient` -- builds remote cluster client for installed CDs.
- `github.com/openshift/hive/pkg/util/scheme` -- `GetScheme()` for client and cache construction.

OpenShift:
- `github.com/openshift/api/machine/v1beta1` -- Machine, MachineSet types.

Kubernetes client/API:
- `k8s.io/api/apps/v1`, `k8s.io/api/core/v1` -- Deployment, Node, Service types.
- `k8s.io/apimachinery/pkg/api/errors`, `k8s.io/apimachinery/pkg/apis/meta/v1`, `k8s.io/apimachinery/pkg/fields`, `k8s.io/apimachinery/pkg/runtime/schema`, `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/dynamic`, `k8s.io/client-go/kubernetes`, `k8s.io/client-go/rest`, `k8s.io/client-go/tools/cache`
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1`, `k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1`
- `sigs.k8s.io/controller-runtime/pkg/cache`, `sigs.k8s.io/controller-runtime/pkg/client`, `sigs.k8s.io/controller-runtime/pkg/client/config`

Third-party:
- `github.com/evanphx/json-patch` -- merge patch computation.
- `github.com/sirupsen/logrus` -- logging.

## Capabilities

- **`package`** name(s): **common**.
- Go **`import`** edges listed below (29 unique path(s)).
- Package ID(s): `github.com/openshift/hive/test/e2e/common`.
- 11 source files, no test files -- this is itself a test helper library.

## Understanding Score

0.85

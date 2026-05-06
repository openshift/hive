# Module atlas

## Responsibility

Shared e2e test helper package providing client construction, resource waiting utilities, cluster deployment retrieval, and JSON diff computation. Used across Hive end-to-end tests to interact with live clusters.

## Public Interface/API

**Client construction (client.go):**
- `MustGetClient() client.WithWatch` -- returns a controller-runtime client using the hive scheme
- `MustGetClientFromConfig(cfg *rest.Config) client.WithWatch` -- same, from explicit config
- `MustGetKubernetesClient() kclient.Interface` -- returns a typed Kubernetes clientset
- `MustGetAPIRegistrationClient() apiregv1client.ApiregistrationV1Interface` -- returns an API registration client
- `MustGetDynamicClient() dynamic.Interface` -- returns a dynamic client
- `MustGetConfig() *rest.Config` -- returns kubeconfig from environment

**Cluster deployment helpers (clusterdeployment.go):**
- `MustGetClusterDeployment() *hivev1.ClusterDeployment` -- fetches CD by CLUSTER_NAME/CLUSTER_NAMESPACE env vars
- `MustGetInstalledClusterDeployment() *hivev1.ClusterDeployment` -- same, but asserts installed state
- `MustGetClusterDeploymentClientConfig() *rest.Config` -- returns REST config for the remote cluster of an installed CD

**Wait helpers:**
- `WaitForDeployment(c kclient.Interface, namespace, name string, testFunc func(*appsv1.Deployment) bool, timeOut time.Duration) error`
- `WaitForDeploymentReady(c kclient.Interface, namespace, name string, timeOut time.Duration) error`
- `WaitForService(c kclient.Interface, namespace, name string, testFunc func(*corev1.Service) bool, timeOut time.Duration) error`
- `WaitForAPIService(c apiregv1client.ApiregistrationV1Interface, name string, testFunc func(*apiregv1.APIService) bool, timeOut time.Duration) error`
- `WaitForAPIServiceAvailable(c apiregv1client.ApiregistrationV1Interface, name string, timeOut time.Duration) error`
- `WaitForMachineSets(cfg *rest.Config, testFunc func([]*machinev1.MachineSet) bool, timeOut time.Duration) error`
- `WaitForMachines(cfg *rest.Config, testFunc func([]*machinev1.Machine) bool, timeOut time.Duration) error`
- `WaitForNodes(cfg *rest.Config, testFunc func([]*corev1.Node) bool, timeOut time.Duration) error`

**Other helpers:**
- `GetMachinePool(c client.Client, cd *hivev1.ClusterDeployment, poolName string) *hivev1.MachinePool` -- fetches a MachinePool by CD name + pool name
- `DynamicWaitForDeletion(dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string, logger log.FieldLogger) error` -- polls until resource is deleted
- `GetHiveNamespaceOrDie() string` -- reads HIVE_NAMESPACE env var
- `GetHiveOperatorNamespaceOrDie() string` -- reads HIVE_OPERATOR_NAMESPACE env var
- `JSONDiff(a, b any) ([]byte, error)` -- computes a JSON merge patch between two objects

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, MachinePool types
- `github.com/openshift/hive/pkg/constants` -- HiveNamespaceEnvVar
- `github.com/openshift/hive/pkg/operator/hive` -- HiveOperatorNamespaceEnvVar
- `github.com/openshift/hive/pkg/remoteclient` -- remote cluster client builder
- `github.com/openshift/hive/pkg/util/scheme` -- shared scheme for client construction
- `github.com/openshift/api/machine/v1beta1` -- Machine, MachineSet types
- `github.com/evanphx/json-patch` -- JSON merge patch
- `k8s.io/client-go/dynamic`, `kubernetes`, `rest` -- Kubernetes clients
- `k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1` -- API registration client
- `sigs.k8s.io/controller-runtime/pkg/client`, `cache`, `config` -- controller-runtime client/cache

## Capabilities

- Construct various Kubernetes client types from kubeconfig
- Wait for Deployments, Services, APIServices, MachineSets, Machines, and Nodes to reach desired state (informer-based watches with timeout)
- Retrieve ClusterDeployment and MachinePool resources for e2e assertions
- Obtain remote cluster REST config from an installed ClusterDeployment
- Compute JSON diffs between arbitrary objects
- Poll for dynamic resource deletion

## Understanding Score

0.85

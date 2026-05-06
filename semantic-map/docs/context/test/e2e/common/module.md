<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`test/e2e/common/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `DynamicWaitForDeletion` — DynamicWaitForDeletion uses the dynamic client to wait for a resource to not exist.
- `GetHiveNamespaceOrDie`
- `GetHiveOperatorNamespaceOrDie`
- `GetMachinePool`
- `JSONDiff`
- `MustGetAPIRegistrationClient`
- `MustGetClient`
- `MustGetClientFromConfig`
- `MustGetClusterDeployment`
- `MustGetClusterDeploymentClientConfig`
- `MustGetConfig`
- `MustGetDynamicClient`
- `MustGetInstalledClusterDeployment`
- `MustGetKubernetesClient`
- `WaitForAPIService`
- `WaitForAPIServiceAvailable`
- `WaitForDeployment`
- `WaitForDeploymentReady`
- `WaitForMachineSets`
- `WaitForMachines`
- `WaitForNodes`
- `WaitForService`

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/evanphx/json-patch`
- `github.com/openshift/api/machine/v1beta1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/operator/hive`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/sirupsen/logrus`
- `k8s.io/api/apps/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/fields`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/client-go/dynamic`
- `k8s.io/client-go/kubernetes`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/cache`
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1`
- `k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1`
- `os`
- `sigs.k8s.io/controller-runtime/pkg/cache`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/config`
- `time`

## Capabilities

- **`package`** name(s): **common**.
- Go **`import`** edges listed below (29 unique path(s)).
- Package ID(s): `github.com/openshift/hive/test/e2e/common`.

## Understanding Score

0.0

<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/clusterdeployment/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new ClusterDeployment controller and adds it to the manager with default RBAC.
- `AddToManager` — AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
- `ClusterProvisionManager`
- `ControllerName`
- `LoadReleaseImageVerifier`
- `NewReconciler` — NewReconciler returns a new reconcile.Reconciler
- `ReconcileClusterDeployment` — ReconcileClusterDeployment reconciles a ClusterDeployment object
- `ReconcileClusterDeployment.Reconcile` — Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read and what is in the ClusterDeployment.Spec
- `ReconcileClusterDeployment.SetWatcher`
- `ValidateInstallConfig` — ValidateInstallConfig ensures that the "install-config.yaml" in the `installConfigSecret` unmarshals correctly as an InstallConfig; validates that its Platform is consistent with …

## Internal Dependencies

- `bytes`
- `context`
- `encoding/json`
- `fmt`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/api/route/v1`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/hive/v1/azure`
- `github.com/openshift/hive/apis/hive/v1/gcp`
- `github.com/openshift/hive/apis/hive/v1/metricsconfig`
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/controller/utils/vsphereutils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/ibmclient`
- `github.com/openshift/hive/pkg/imageset`
- `github.com/openshift/hive/pkg/install`
- `github.com/openshift/hive/pkg/remoteclient`
- `github.com/openshift/hive/pkg/util/contracts`
- `github.com/openshift/hive/pkg/util/labels`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/aws`
- `github.com/openshift/installer/pkg/types/azure`
- `github.com/openshift/installer/pkg/types/gcp`
- `github.com/openshift/installer/pkg/types/ibmcloud`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/openstack`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/openshift/library-go/pkg/controller`
- `github.com/openshift/library-go/pkg/manifest`
- `github.com/openshift/library-go/pkg/verify`
- `github.com/openshift/library-go/pkg/verify/store/sigstore`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/rand`
- `k8s.io/client-go/dynamic`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/utils/ptr`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sigs.k8s.io/yaml`
- `sort`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **clusterdeployment**.
- Go **`import`** edges listed below (71 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/clusterdeployment`.

## Understanding Score

0.0

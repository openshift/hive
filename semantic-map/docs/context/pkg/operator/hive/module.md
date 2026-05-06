<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/operator/hive/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Add` — Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller and Start it when the Manager is Started.
- `ControllerName`
- `FindHiveConfigCondition`
- `GetHiveNamespace`
- `HiveConfigConditions`
- `HiveOperatorNamespaceEnvVar`
- `InitializeHiveConfigConditions` — InitializeHiveConfigConditions initializes the given set of conditions for the first time, set with Status Unknown
- `ReconcileHiveConfig` — ReconcileHiveConfig reconciles a Hive object
- `ReconcileHiveConfig.Get`
- `ReconcileHiveConfig.List`
- `ReconcileHiveConfig.Reconcile` — Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read and what is in the Hive.Spec
- `SetHiveConfigCondition`

## Internal Dependencies

- `bytes`
- `context`
- `crypto/md5`
- `encoding/hex`
- `encoding/json`
- `fmt`
- `github.com/openshift/api/apps/v1`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/client-go/config/listers/config/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/operator/assets`
- `github.com/openshift/hive/pkg/operator/metrics`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/contracts`
- `github.com/openshift/hive/pkg/util/logrus`
- `github.com/openshift/hive/pkg/util/scheme`
- `github.com/openshift/library-go/pkg/operator/configobserver/apiserver`
- `github.com/openshift/library-go/pkg/operator/resource/resourceread`
- `github.com/openshift/library-go/pkg/operator/resourcesynccontroller`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/admissionregistration/v1`
- `k8s.io/api/apps/v1`
- `k8s.io/api/core/v1`
- `k8s.io/api/rbac/v1`
- `k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/labels`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/runtime/serializer`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/discovery`
- `k8s.io/client-go/dynamic`
- `k8s.io/client-go/informers`
- `k8s.io/client-go/kubernetes`
- `k8s.io/client-go/listers/core/v1`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/cache`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1`
- `k8s.io/utils/ptr`
- `os`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/predicate`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `slices`
- `sort`
- `strconv`
- `strings`
- `time`

## Capabilities

- **`package`** name(s): **hive**.
- Go **`import`** edges listed below (67 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/operator/hive`.

## Understanding Score

0.0

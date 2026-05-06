<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/validating-webhooks/hive/v1/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ClusterDeploymentCustomizationValidatingAdmissionHook` — ClusterDeploymentCustomizationlValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `ClusterDeploymentCustomizationValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `ClusterDeploymentCustomizationValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `ClusterDeploymentCustomizationValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `ClusterDeploymentValidatingAdmissionHook` — ClusterDeploymentValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `ClusterDeploymentValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `ClusterDeploymentValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `ClusterDeploymentValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `ClusterImageSetValidatingAdmissionHook` — ClusterImageSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `ClusterImageSetValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `ClusterImageSetValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `ClusterImageSetValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `ClusterPoolValidatingAdmissionHook` — ClusterPoolValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `ClusterPoolValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `ClusterPoolValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `ClusterPoolValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `ClusterProvisionValidatingAdmissionHook` — ClusterProvisionValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `ClusterProvisionValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `ClusterProvisionValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `ClusterProvisionValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `DNSZoneValidatingAdmissionHook` — DNSZoneValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `DNSZoneValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `DNSZoneValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `DNSZoneValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `MachinePoolValidatingAdmissionHook` — MachinePoolValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `MachinePoolValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `MachinePoolValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `MachinePoolValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `SelectorSyncSetValidatingAdmissionHook` — SelectorSyncSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `SelectorSyncSetValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `SelectorSyncSetValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `SelectorSyncSetValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…
- `SyncSetValidatingAdmissionHook` — SyncSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
- `SyncSetValidatingAdmissionHook.Initialize` — Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
- `SyncSetValidatingAdmissionHook.Validate` — Validate is called by generic-admission-server when the registered REST resource above is called with an admission request. Usually it's the kube apiserver that is making the admi…
- `SyncSetValidatingAdmissionHook.ValidatingResource` — ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the webhook is accessed by the kube apiserver. For example…

## Internal Dependencies

- `encoding/json`
- `fmt`
- `github.com/google/go-cmp/cmp`
- `github.com/google/go-cmp/cmp/cmpopts`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/hive/v1/azure`
- `github.com/openshift/hive/apis/hive/v1/gcp`
- `github.com/openshift/hive/apis/hive/v1/ibmcloud`
- `github.com/openshift/hive/apis/hive/v1/nutanix`
- `github.com/openshift/hive/apis/hive/v1/openstack`
- `github.com/openshift/hive/apis/hive/v1/vsphere`
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/awsprivatelink`
- `github.com/openshift/hive/pkg/manageddns`
- `github.com/openshift/hive/pkg/util/contracts`
- `github.com/sirupsen/logrus`
- `github.com/stretchr/testify/assert`
- `k8s.io/api/admission/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/validation`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/apis/meta/v1/validation`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/apimachinery/pkg/util/validation`
- `k8s.io/apimachinery/pkg/util/validation/field`
- `k8s.io/client-go/rest`
- `net/http`
- `os`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/webhook/admission`
- `strconv`
- `strings`

## Capabilities

- **`package`** name(s): **v1**.
- Go **`import`** edges listed below (37 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/validating-webhooks/hive/v1`.

## Understanding Score

0.0

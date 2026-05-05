# Module atlas

## Responsibility

Implements **Kubernetes validating admission webhooks** for all major Hive CRD types. Each webhook struct follows a consistent pattern: it registers itself under the `admission.hive.openshift.io/v1` API group, validates Create/Update (and sometimes Delete) operations by decoding the admission request into typed Hive API objects, and returns allow/deny responses with detailed error messages.

The webhooks enforce:
- **ClusterDeployment** -- name length, platform configuration (AWS/Azure/GCP/OpenStack/vSphere/IBMCloud/Nutanix/BareMetal), managed DNS domain validation, ingress and certificate bundle rules, immutability of most spec fields after creation (with enumerated mutable exceptions), ClusterPoolRef lifecycle rules, protected-delete annotation, AWS PrivateLink region support, ClusterInstall contract support, and feature gate gating.
- **ClusterDeploymentCustomization** -- name length validation on create.
- **ClusterImageSet** -- release image is required, digest format validation, and optional digest-only enforcement when release verification is enabled.
- **ClusterPool** -- name length and platform configuration validation (reuses shared `validatePlatformConfiguration`).
- **ClusterProvision** -- stage transition validation (initializing->provisioning->complete/failed), immutability of clusterDeploymentRef/podSpec/attempt/clusterID/infraID after create, and invariant checks on secrets and IDs.
- **DNSZone** -- zone name must be valid DNS subdomain, zone field is immutable on update.
- **MachinePool** -- name must follow `${CD_NAME}-${POOL_NAME}` convention, platform-specific field validation for all cloud providers (instance types, zones, disk sizes, CPU/memory), autoscaling min/max rules, label validation, and immutability of clusterDeploymentRef/name/platform on update.
- **SyncSet / SelectorSyncSet** -- validates that embedded resources are valid JSON with no forbidden OpenShift authorization API group kinds, patch types are valid, secret mappings have names, SyncSet source secrets are in the same namespace, and resourceApplyMode is valid.

Supporting internals include a `featureSet` type that reads enabled feature gates from the `HIVE_FEATURE_GATES_ENABLED` environment variable, and shared validation helpers for platform configuration and ingress.

## Public Interface/API

Each webhook type exports three methods required by the generic-admission-server framework:
- `ValidatingResource() (schema.GroupVersionResource, string)` -- registers the webhook's REST resource.
- `Initialize(*rest.Config, <-chan struct{}) error` -- startup hook (currently no-ops for all webhooks).
- `Validate(*admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse` -- main validation entry point.

Webhook types and their constructors:
- `ClusterDeploymentValidatingAdmissionHook` / `NewClusterDeploymentValidatingAdmissionHook(decoder)`
- `ClusterDeploymentCustomizationValidatingAdmissionHook` / `NewClusterDeploymentCustomizationValidatingAdmissionHook(decoder)`
- `ClusterImageSetValidatingAdmissionHook` / `NewClusterImageSetValidatingAdmissionHook(decoder)`
- `ClusterPoolValidatingAdmissionHook` / `NewClusterPoolValidatingAdmissionHook(decoder)`
- `ClusterProvisionValidatingAdmissionHook` / `NewClusterProvisionValidatingAdmissionHook(decoder)`
- `DNSZoneValidatingAdmissionHook` / `NewDNSZoneValidatingAdmissionHook(decoder)`
- `MachinePoolValidatingAdmissionHook` / `NewMachinePoolValidatingAdmissionHook(decoder)`
- `SyncSetValidatingAdmissionHook` / `NewSyncSetValidatingAdmissionHook(decoder)`
- `SelectorSyncSetValidatingAdmissionHook` / `NewSelectorSyncSetValidatingAdmissionHook(decoder)`

## Internal Dependencies

Hive APIs and internal packages:
- `github.com/openshift/hive/apis/hive/v1` and platform sub-packages (`aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere`)
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1` -- ClusterInstall contract name constant
- `github.com/openshift/hive/pkg/constants` -- env vars, annotations, labels for feature gates, protected delete, DR labels, release image verification
- `github.com/openshift/hive/pkg/controller/awsprivatelink` -- reads AWS PrivateLink controller config
- `github.com/openshift/hive/pkg/manageddns` -- reads managed DNS domains file
- `github.com/openshift/hive/pkg/util/contracts` -- contract implementation support checks

Kubernetes and ecosystem:
- `k8s.io/api/admission/v1`, `k8s.io/apimachinery/pkg/api/errors`, `k8s.io/apimachinery/pkg/api/validation`, `k8s.io/apimachinery/pkg/apis/meta/v1`, `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`, `k8s.io/apimachinery/pkg/apis/meta/v1/validation`, `k8s.io/apimachinery/pkg/runtime`, `k8s.io/apimachinery/pkg/runtime/schema`, `k8s.io/apimachinery/pkg/util/sets`, `k8s.io/apimachinery/pkg/util/validation`, `k8s.io/apimachinery/pkg/util/validation/field`, `k8s.io/client-go/rest`
- `sigs.k8s.io/controller-runtime/pkg/webhook/admission`
- `github.com/google/go-cmp/cmp`, `github.com/google/go-cmp/cmp/cmpopts` -- immutability diff reporting
- `github.com/sirupsen/logrus`

Test dependencies: `github.com/stretchr/testify/assert` (used in feature_gates.go and test files)

## Capabilities

- **`package`** name(s): **v1**.
- Go **`import`** edges listed below (37 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/validating-webhooks/hive/v1`.
- 20 source files total: 10 webhook/helper files and 10 test files providing extensive coverage.
- Shared validation logic: `validatePlatformConfiguration` is used by both ClusterDeployment and ClusterPool webhooks.

## Understanding Score

0.8

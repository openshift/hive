# Module atlas

## Responsibility

Implements validating admission webhooks for Hive custom resources. Each webhook hook struct registers a REST resource under `admission.hive.openshift.io/v1`, implements `Initialize`, `ValidatingResource`, and `Validate` methods, and enforces field-level validation rules on CREATE/UPDATE operations.

## Public Interface/API

Each hook type exposes the same three-method interface (`ValidatingResource`, `Initialize`, `Validate`):

- `NewClusterDeploymentValidatingAdmissionHook(decoder admission.Decoder) *ClusterDeploymentValidatingAdmissionHook` -- validates ClusterDeployment resources; reads managed domains, feature gates, AWS private link config, and supported contracts
- `NewClusterDeploymentCustomizationValidatingAdmissionHook(decoder admission.Decoder) *ClusterDeploymentCustomizationValidatingAdmissionHook` -- validates ClusterDeploymentCustomization resources
- `NewClusterImageSetValidatingAdmissionHook(decoder admission.Decoder) *ClusterImageSetValidatingAdmissionHook` -- validates ClusterImageSet resources
- `NewClusterPoolValidatingAdmissionHook(decoder admission.Decoder) *ClusterPoolValidatingAdmissionHook` -- validates ClusterPool resources
- `NewClusterProvisionValidatingAdmissionHook(decoder admission.Decoder) *ClusterProvisionValidatingAdmissionHook` -- validates ClusterProvision resources (stage transitions)
- `NewDNSZoneValidatingAdmissionHook(decoder admission.Decoder) *DNSZoneValidatingAdmissionHook` -- validates DNSZone resources
- `NewMachinePoolValidatingAdmissionHook(decoder admission.Decoder) *MachinePoolValidatingAdmissionHook` -- validates MachinePool resources (platform-specific rules for AWS, GCP, Azure, IBMCloud, Nutanix, OpenStack, vSphere)
- `NewSelectorSyncSetValidatingAdmissionHook(decoder admission.Decoder) *SelectorSyncSetValidatingAdmissionHook` -- validates SelectorSyncSet resources
- `NewSyncSetValidatingAdmissionHook(decoder admission.Decoder) *SyncSetValidatingAdmissionHook` -- validates SyncSet resources (resource apply modes, patch types, forbidden resource groups)

Internal helpers:
- `featureSet` -- reads enabled feature gates from env var and provides `IsEnabled(featureGate string) bool`
- `existsOnlyWhenFeatureGate(...)` / `equalOnlyWhenFeatureGate(...)` -- field-level feature gate validation utilities

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- all Hive CRD types
- `github.com/openshift/hive/apis/hive/v1/aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere` -- platform-specific types
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1` -- contract types
- `github.com/openshift/hive/pkg/constants` -- env vars and constants
- `github.com/openshift/hive/pkg/controller/awsprivatelink` -- AWS private link config reading
- `github.com/openshift/hive/pkg/manageddns` -- managed domains file reading
- `github.com/openshift/hive/pkg/util/contracts` -- contract implementation lookup
- `k8s.io/api/admission/v1` -- AdmissionRequest/AdmissionResponse
- `k8s.io/apimachinery/pkg/runtime/schema` -- GVR registration
- `k8s.io/client-go/rest` -- webhook initialization config
- `sigs.k8s.io/controller-runtime/pkg/webhook/admission` -- Decoder

## Capabilities

- Validate CREATE and UPDATE operations on 9 Hive CRD types
- Enforce immutability rules on ClusterDeployment fields (mutableFields allowlist)
- Validate DNS names, label selectors, platform-specific MachinePool configuration
- Enforce provision stage transition rules on ClusterProvision
- Validate SyncSet/SelectorSyncSet resource apply modes, patch types, and forbidden resource groups
- Feature-gate-aware field validation
- Managed domain validation on ClusterDeployment

## Understanding Score

0.85

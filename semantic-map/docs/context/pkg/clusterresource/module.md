<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/clusterresource/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSCloudBuilder` — AWSCloudBuilder encapsulates cluster artifact generation logic specific to AWS.
- `AWSCloudBuilder.CredsSecretName`
- `AWSCloudBuilder.GenerateCloudObjects`
- `AWSCloudBuilder.GenerateCredentialsSecret`
- `AWSCloudBuilder.GetCloudPlatform`
- `AWSInstanceTypeDefault`
- `AzureCloudBuilder` — AzureCloudBuilder encapsulates cluster artifact generation logic specific to Azure.
- `AzureCloudBuilder.CredsSecretName`
- `AzureCloudBuilder.GenerateCloudObjects`
- `AzureCloudBuilder.GenerateCredentialsSecret`
- `AzureCloudBuilder.GetCloudPlatform`
- `Builder` — Builder can be used to build all artifacts required for to create a ClusterDeployment.
- `Builder.Build` — Build generates all resources using the fields configured.
- `Builder.GeneratePullSecretSecret` — GeneratePullSecretSecret returns a Kubernetes Secret containing the pull secret to be used for pulling images.
- `Builder.GetPullSecretSecretName` — TODO: handle long cluster names.
- `Builder.Validate` — Validate ensures that the builder's fields are logically configured and usable to generate the cluster resources.
- `CloudBuilder` — CloudBuilder interface exposes the functions we will use to set cloud specific portions of the cluster's resources.
- `GCPCloudBuilder` — GCPCloudBuilder encapsulates cluster artifact generation logic specific to GCP.
- `GCPCloudBuilder.CredsSecretName`
- `GCPCloudBuilder.GenerateCloudObjects`
- `GCPCloudBuilder.GenerateCredentialsSecret`
- `GCPCloudBuilder.GetCloudPlatform`
- `IBMCloudBuilder` — IBMCloudBuilder encapsulates cluster artifact generation logic specific to IBM Cloud.
- `IBMCloudBuilder.CredsSecretName`
- `IBMCloudBuilder.GenerateCloudObjects`
- `IBMCloudBuilder.GenerateCredentialsSecret`
- `IBMCloudBuilder.GetCloudPlatform`
- `InstallConfigTemplate` — InstallConfigTemplate allows for overlaying generic InstallConfig with parts known to Hive
- `InstallConfigTemplate.MarshalJSON` — MarshalJSON will merge the known fields from InstallConfigTemplate
- `InstallConfigTemplate.UnmarshalJSON` — UnmarshalJSON will extract the known types in InstallConfigTemplate
- `NutanixCloudBuilder`
- `NutanixCloudBuilder.CredsSecretName`
- `NutanixCloudBuilder.GenerateCloudObjects`
- `NutanixCloudBuilder.GenerateCredentialsSecret`
- `NutanixCloudBuilder.GetCloudPlatform`
- `OpenStackCloudBuilder` — OpenStackCloudBuilder encapsulates cluster artifact generation logic specific to OpenStack.
- `OpenStackCloudBuilder.CredsSecretName`
- `OpenStackCloudBuilder.GenerateCloudObjects`
- `OpenStackCloudBuilder.GenerateCredentialsSecret`
- `OpenStackCloudBuilder.GetCloudPlatform`
- `VSphereCloudBuilder` — VSphereCloudBuilder encapsulates cluster artifact generation logic specific to vSphere.
- `VSphereCloudBuilder.CredsSecretName`
- `VSphereCloudBuilder.GenerateCloudObjects`
- `VSphereCloudBuilder.GenerateCredentialsSecret`
- `VSphereCloudBuilder.GetCloudPlatform`
- `VSphereCloudBuilder.Infrastructure` — Returns the VSphere Platform (install-config style) of the builder. If `clean` is true, we will scrub the credentials out of it. Do this e.g. if injecting into a non-Secret CR, bu…

## Internal Dependencies

- `encoding/json`
- `fmt`
- `github.com/openshift/api/config/v1`
- `github.com/openshift/api/machine/v1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/hive/v1/azure`
- `github.com/openshift/hive/apis/hive/v1/gcp`
- `github.com/openshift/hive/apis/hive/v1/ibmcloud`
- `github.com/openshift/hive/apis/hive/v1/nutanix`
- `github.com/openshift/hive/apis/hive/v1/openstack`
- `github.com/openshift/hive/apis/hive/v1/vsphere`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/utils/nutanixutils`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/util/yaml`
- `github.com/openshift/installer/pkg/ipnet`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/aws`
- `github.com/openshift/installer/pkg/types/azure`
- `github.com/openshift/installer/pkg/types/gcp`
- `github.com/openshift/installer/pkg/types/ibmcloud`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/openstack`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/openshift/installer/pkg/validate`
- `github.com/pkg/errors`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`
- `sigs.k8s.io/yaml`
- `time`

## Capabilities

- **`package`** name(s): **clusterresource**.
- Go **`import`** edges listed below (33 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/clusterresource`.

## Understanding Score

0.0

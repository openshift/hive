# Module atlas

## Responsibility

Provides a builder library for generating all Kubernetes resources required to create a Hive ClusterDeployment, including install-config secrets, credentials secrets, machine pools, and cloud-specific objects for AWS, Azure, GCP, IBM Cloud, OpenStack, vSphere, and Nutanix. Tests exist.

## Public Interface/API

**Interfaces:**
- `CloudBuilder` -- Interface for cloud-specific resource generation: `GetCloudPlatform`, `CredsSecretName`, `GenerateCredentialsSecret`, `GenerateCloudObjects`, plus unexported `addMachinePoolPlatform`, `addInstallConfigPlatform`

**Types:**
- `Builder` -- Central builder struct with fields for cluster name, namespace, cloud builder, pull secret, SSH keys, credentials mode, adoption settings, installer manifests, image sets, etc.
- `AWSCloudBuilder` -- AWS-specific builder (access key, secret key, region, instance types, user tags, PrivateLink)
- `AzureCloudBuilder` -- Azure-specific builder (service principal, region, cloud name, resource groups)
- `GCPCloudBuilder` -- GCP-specific builder (service account, project ID, region, Private Service Connect, SSD hibernate behavior)
- `IBMCloudBuilder` -- IBM Cloud-specific builder (API key, region, instance type)
- `NutanixCloudBuilder` -- Nutanix-specific builder (Prism Central, failure domains, VIPs, CA cert)
- `OpenStackCloudBuilder` -- OpenStack-specific builder (cloud name, clouds.yaml, network, flavors, floating IPs)
- `VSphereCloudBuilder` -- vSphere-specific builder (credentials data, CA cert, infrastructure platform spec)
- `InstallConfigTemplate` -- Overlay struct for merging user-supplied install-config templates with Hive-known fields

**Key Functions:**
- `(o *Builder) Build() ([]runtime.Object, error)` -- Generates all ClusterDeployment, MachinePool, install-config Secret, credentials Secret, and cloud objects
- `(o *Builder) Validate() error` -- Validates builder configuration
- `(o *Builder) GeneratePullSecretSecret() *corev1.Secret` -- Generates pull secret Secret
- `(o *Builder) GetPullSecretSecretName() string` -- Returns pull secret name
- `NewAWSCloudBuilderFromSecret(secret) *AWSCloudBuilder` -- Create AWS builder from Secret
- `NewAWSCloudBuilderFromAssumeRole(role) *AWSCloudBuilder` -- Create AWS builder from AssumeRole
- `NewAzureCloudBuilderFromSecret(secret) *AzureCloudBuilder` -- Create Azure builder from Secret
- `NewGCPCloudBuilderFromSecret(secret) (*GCPCloudBuilder, error)` -- Create GCP builder from Secret
- `NewOpenStackCloudBuilderFromSecret(secret) *OpenStackCloudBuilder` -- Create OpenStack builder from Secret
- `NewVSphereCloudBuilder(creds, certs, infra) *VSphereCloudBuilder` -- Create vSphere builder
- `(b *VSphereCloudBuilder) Infrastructure(clean bool) *installervsphere.Platform` -- Returns platform spec, optionally scrubbed of credentials

**Constants:**
- `AWSInstanceTypeDefault` = `"m6a.xlarge"`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- Hive CRD types (ClusterDeployment, MachinePool, Platform, etc.)
- `github.com/openshift/hive/apis/hive/v1/aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere` -- Per-cloud platform types
- `github.com/openshift/hive/pkg/constants` -- Secret keys, file names
- `github.com/openshift/hive/pkg/controller/utils/nutanixutils` -- Nutanix failure domain conversion
- `github.com/openshift/hive/pkg/gcpclient` -- GCP project ID extraction
- `github.com/openshift/hive/pkg/util/yaml` -- JSON patch application
- `github.com/openshift/installer/pkg/types` -- InstallConfig, CredentialsMode
- `github.com/openshift/installer/pkg/types/aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere` -- Installer platform types
- `github.com/openshift/api/config/v1` -- FeatureSet type
- `github.com/openshift/api/machine/v1` -- Nutanix boot type

## Capabilities

- Generate complete set of Kubernetes resources for ClusterDeployment creation (CD, MachinePool, install-config Secret, credentials Secret, SSH key Secret, pull secret Secret, serving cert Secret, manifests Secret, bound SA signing key Secret)
- Support 7 cloud providers with provider-specific install-config platform injection and credential handling
- Support cluster adoption with admin kubeconfig, metadata.json, and admin password secrets
- Merge user-provided install-config templates with Hive-managed fields
- Validate builder configuration before resource generation
- Apply JSON patches to remove unsupported fields from install-config (e.g., metadataService for AWS, osImage for Azure)

## Understanding Score

0.9

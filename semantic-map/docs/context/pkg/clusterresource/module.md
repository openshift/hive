# Module atlas

## Responsibility

Generates the complete set of Kubernetes resources needed to create a ClusterDeployment: the ClusterDeployment CR, MachinePool, install-config Secret, credential Secrets, and pull secret Secret. Supports all 7 cloud platforms via the `CloudBuilder` interface.

## Public Interface/API

- `Builder` — configurable struct: Name, Namespace, CloudBuilder, PullSecret, SSHKeys, BaseDomain, WorkerNodesCount, ManageDNS, HibernateAfter, etc.
- `Builder.Build() ([]runtime.Object, error)` — generates all resources
- `Builder.Validate() error` — validates configuration (SSH key, base domain, etc.)
- `Builder.GeneratePullSecretSecret() *corev1.Secret`
- `CloudBuilder` — interface: `GetCloudPlatform()`, `GenerateCredentialsSecret()`, `GenerateCloudObjects()`, `CredsSecretName()`
- `AWSCloudBuilder`, `AzureCloudBuilder`, `GCPCloudBuilder`, `OpenStackCloudBuilder`, `VSphereCloudBuilder`, `NutanixCloudBuilder`, `IBMCloudBuilder` — per-platform implementations
- `InstallConfigTemplate` — JSON merge overlay for install-config customization

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment, MachinePool, platform types
- `github.com/openshift/hive/pkg/constants` — credential key names, platform constants
- `github.com/openshift/hive/pkg/controller/utils/nutanixutils` — Nutanix credential helpers
- `github.com/openshift/hive/pkg/gcpclient` — GCP service account key extraction
- `github.com/openshift/hive/pkg/util/yaml` — YAML merge patching for install-config
- `github.com/openshift/installer/pkg/types` — InstallConfig and per-platform types (external)

## Capabilities

- Generates ClusterDeployment, MachinePool, install-config Secret, credential Secrets, and pull secret Secret
- Supports 7 platforms: AWS, Azure, GCP, OpenStack, vSphere, Nutanix, IBM Cloud
- Configurable networking (machine CIDR, service CIDR, pod CIDR), worker count, SSH keys
- Install-config template overlay via JSON merge for user-provided base configs
- Validates SSH public keys and base domain format

## Understanding Score

0.8

# Module atlas

## Responsibility

Implements the `create-cluster` command for hiveutil: generates and optionally applies a complete set of ClusterDeployment resources for provisioning an OpenShift cluster across all supported cloud platforms.

## Public Interface/API

- `NewCreateClusterCommand() *cobra.Command` — generates and applies cluster deployment artifacts
- `Options` — large exported options struct covering all platforms (AWS, Azure, GCP, OpenStack, vSphere, Nutanix, IBM Cloud)
- `Options.Complete(cmd, args) error` — finalizes options from flags, env, and credential files
- `Options.Validate(cmd) error` — validates option combinations per platform
- `Options.GenerateObjects() ([]runtime.Object, error)` — produces ClusterDeployment, MachinePool, install-config Secret, cred Secrets, etc.
- `Options.Run() error` — calls GenerateObjects then applies or prints the resources

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment, MachinePool, Platform types
- `github.com/openshift/hive/apis/hive/v1/azure` — Azure cloud configuration types
- `github.com/openshift/hive/contrib/pkg/utils` — GetClient, GetResourceHelper
- `github.com/openshift/hive/pkg/clusterresource` — ClusterResource builder for install-config generation
- `github.com/openshift/hive/pkg/constants` — platform constants
- `github.com/openshift/hive/pkg/creds/{aws,azure,gcp,openstack}` — platform credential loading
- `github.com/openshift/hive/pkg/gcpclient` — GCP service account key parsing
- `github.com/openshift/installer/pkg/types` — InstallConfig and platform-specific types
- `github.com/openshift/installer/pkg/validate` — SSH key validation

## Capabilities

- Generates a complete ClusterDeployment manifest set (ClusterDeployment, MachinePool, install-config Secret, credential Secrets)
- Supports 7 platforms: AWS, Azure, GCP, OpenStack, vSphere, Nutanix, IBM Cloud
- Supports dry-run YAML/JSON output or direct apply to cluster
- Configurable: release image, base domain, worker count, networking (machine CIDR, service CIDR), serving certs, manifests injection
- Can adopt existing clusters via `--adopt-admin-kubeconfig`

## Understanding Score

0.8

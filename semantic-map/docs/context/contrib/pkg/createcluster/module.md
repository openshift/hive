# Module atlas

## Responsibility

Provides a CLI command (`create-cluster`) for the `hiveutil` tool that generates and applies all Kubernetes resources needed to create a new Hive ClusterDeployment, supporting AWS, Azure, GCP, IBM Cloud, OpenStack, vSphere, and Nutanix cloud providers.

## Public Interface/API

**Types:**
- `Options` -- Configuration struct holding all flags and parameters for cluster creation (cloud provider, credentials, region, SSH keys, pull secret, adoption settings, per-cloud options for AWS/Azure/GCP/OpenStack/vSphere/IBM/Nutanix)

**Functions:**
- `NewCreateClusterCommand() *cobra.Command` -- Creates the `create-cluster` cobra command with all cloud-specific flags
- `(o *Options) Complete(cmd *cobra.Command, args []string) error` -- Finishes parsing arguments, sets region defaults, parses durations
- `(o *Options) Validate(cmd *cobra.Command) error` -- Validates flag combinations and cloud-specific requirements
- `(o *Options) Run() error` -- Generates objects and either prints (yaml/json) or applies them to the cluster
- `(o *Options) GenerateObjects() ([]runtime.Object, error)` -- Generates all ClusterDeployment, Secret, MachinePool, ImageSet, and SyncSet resources

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- Hive CRD types
- `github.com/openshift/hive/apis/hive/v1/azure` -- Azure platform types
- `github.com/openshift/hive/contrib/pkg/utils` -- Kube client, pull secret, resource helper utilities
- `github.com/openshift/hive/pkg/clusterresource` -- Builder and CloudBuilder for generating cluster resources
- `github.com/openshift/hive/pkg/constants` -- Platform names, env vars, secret keys
- `github.com/openshift/hive/pkg/creds/aws`, `azure`, `gcp`, `openstack` -- Credential loaders per cloud
- `github.com/openshift/hive/pkg/gcpclient` -- GCP project ID extraction
- `github.com/openshift/hive/pkg/util/scheme` -- Scheme registration
- `github.com/openshift/installer/pkg/types`, `vsphere`, `vsphere/conversion`, `nutanix` -- Installer platform types
- `github.com/openshift/installer/pkg/validate` -- CA bundle validation for additional trust bundles
- `github.com/spf13/cobra` -- CLI framework

## Capabilities

- Generate ClusterDeployment, MachinePool, install-config Secret, credentials Secret, SSH key Secret, pull secret Secret, serving cert Secret, ClusterImageSet, and SyncSet/SelectorSyncSet resources
- Support 7 cloud providers: AWS, Azure, GCP, IBM Cloud, OpenStack, vSphere, Nutanix
- Support cluster adoption (importing pre-existing clusters) with admin kubeconfig, infra ID, cluster ID
- Output generated resources as YAML or JSON, or apply directly to the cluster
- Handle credential loading from files, environment variables, or Kubernetes secrets per cloud provider
- Manage optional features: DNS management, hibernation, deletion scheduling, PrivateLink, manual CCO mode, bound SA signing keys, additional trust bundles, installer manifests injection

## Understanding Score

0.9

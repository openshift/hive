# Module atlas

## Responsibility

Implements the `hiveutil clusterpool` subcommand tree for creating ClusterPool resources and claiming clusters from existing pools, supporting AWS, Azure, and GCP cloud providers.

## Public Interface/API

**Types:**
- `type ClusterPoolOptions struct` — options for the `create-pool` subcommand (name, namespace, cloud, region, size, image set, pull secret, hibernate settings, output format)
- `type ClusterClaimOptions struct` — options for the `claim` subcommand (name, namespace, lifetime, cluster pool name)

**Functions:**
- `func NewClusterPoolCommand() *cobra.Command` — parent `clusterpool` command; registers `create-pool` and `claim` subcommands
- `func NewCreateClusterPoolCommand() *cobra.Command` — `create-pool CLUSTER_POOL_NAME`; generates and applies ClusterPool, credentials secret, pull secret, and ClusterImageSet resources. Flags: `--cloud`, `--namespace`, `--base-domain`, `--pull-secret`, `--pull-secret-file`, `--creds-file`, `--cloud-secret`, `--image-set`, `--release-image`, `--release-image-source`, `--region`, `--size`, `--azure-base-domain-resource-group-name`, `--hibernate-after`, `--output`
- `func NewClaimClusterPoolCommand() *cobra.Command` — `claim CLUSTER_POOL_NAME CLAIM_NAME`; creates a ClusterClaim against an existing pool. Flags: `--namespace`, `--lifetime`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterPool, ClusterClaim, ClusterImageSet types
- `github.com/openshift/hive/contrib/pkg/utils` — `GetResourceHelper`, `DefaultNamespace`, `GetPullSecret`, `DetermineReleaseImageFromSource`
- `github.com/openshift/hive/pkg/clusterresource` — Builder, AWSCloudBuilder, AzureCloudBuilder, GCPCloudBuilder for resource generation
- `github.com/openshift/hive/pkg/constants` — platform name constants
- `github.com/openshift/hive/pkg/creds/aws` — `GetAWSCreds`
- `github.com/openshift/hive/pkg/creds/azure` — `GetCreds`
- `github.com/openshift/hive/pkg/creds/gcp` — `GetCreds`
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/pkg/errors` — error wrapping
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `k8s.io/cli-runtime/pkg/printers` — YAML/JSON output printers

## Capabilities

- Creates ClusterPool resources with associated credentials secrets, pull secret secrets, and ClusterImageSets
- Supports AWS, Azure, and GCP cloud providers with per-cloud credential extraction and builder configuration
- Can determine release image from a URL source if not explicitly provided
- Supports dry-run output in YAML or JSON format (`--output` flag)
- Creates ClusterClaim resources to claim clusters from an existing pool, with optional lifetime
- Defaults cloud region per provider (us-east-1 for AWS, centralus for Azure, us-east1 for GCP)

## Understanding Score

0.9

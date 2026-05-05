# Module atlas

## Responsibility

Implements `clusterpool` subcommands for hiveutil: creating ClusterPool resources and claiming clusters from existing pools via ClusterClaim objects.

## Public Interface/API

- `NewClusterPoolCommand() *cobra.Command` — top-level `clusterpool` command grouping create-pool and claim
- `NewCreateClusterPoolCommand() *cobra.Command` — creates a ClusterPool with cloud credentials and install-config
- `NewClaimClusterPoolCommand() *cobra.Command` — creates a ClusterClaim against a named pool
- `ClusterPoolOptions` — exported options for pool creation (cloud creds, platform, size, pull secret, etc.)
- `ClusterClaimOptions` — exported options for claiming a cluster from a pool

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterPool, ClusterClaim types
- `github.com/openshift/hive/contrib/pkg/utils` — GetClient, shared helpers
- `github.com/openshift/hive/pkg/clusterresource` — ClusterResource builder for install-config generation
- `github.com/openshift/hive/pkg/constants` — platform constants
- `github.com/openshift/hive/pkg/creds/{aws,azure,gcp}` — cloud credential loading
- `github.com/openshift/hive/pkg/util/scheme` — shared scheme registration

## Capabilities

- Creates ClusterPool resources with configurable size, cloud credentials, and install-config
- Generates required Secrets (cloud creds, pull secret, install-config) alongside the pool
- Claims a cluster from an existing pool by creating a ClusterClaim CR
- Supports dry-run output via `k8s.io/cli-runtime` printers

## Understanding Score

0.8

# Module atlas

## Responsibility

Implements the `cluster-report` command group for hiveutil: generates provisioning and deprovisioning reports by querying ClusterDeployment resources and their associated conditions and metadata.

## Public Interface/API

- `NewClusterReportCommand() *cobra.Command` — top-level `cluster-report` command with provisioning and deprovisioning subcommands
- `NewProvisioningReportCommand() *cobra.Command` — reports on clusters currently provisioning or recently provisioned
- `NewDeprovisioningReportCommand() *cobra.Command` — reports on clusters currently deprovisioning
- `ProvisioningReportOptions` — options for provisioning report (namespace filter, time thresholds)
- `ProvisioningReportOptions.Complete/Validate/Run`
- `DeprovisioningReportOptions` — options for deprovisioning report
- `DeprovisioningReportOptions.Complete/Validate/Run`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment, conditions
- `github.com/openshift/hive/contrib/pkg/utils` — GetClient
- `sigs.k8s.io/controller-runtime/pkg/client` — dynamic client for listing ClusterDeployments

## Capabilities

- Lists ClusterDeployments and reports provisioning status, timing, and conditions
- Lists deprovisioning clusters with their status and age
- Supports namespace filtering or cluster-wide queries
- Outputs tabular reports to stdout

## Understanding Score

0.7

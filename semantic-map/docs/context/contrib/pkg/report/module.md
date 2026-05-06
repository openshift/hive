# Module atlas

## Responsibility

Provides CLI commands for the `hiveutil` tool to generate reports on clusters managed by Hive, including provisioning and deprovisioning status reports with filtering capabilities.

## Public Interface/API

**Types:**
- `ProvisioningReportOptions` -- Options for the provisioning report (age filters, cluster type filter)
- `DeprovisioningReportOptions` -- Options for the deprovisioning report (cluster type filter)

**Functions:**
- `NewClusterReportCommand() *cobra.Command` -- Top-level `report` command that aggregates provisioning and deprovisioning report subcommands
- `NewProvisioningReportCommand() *cobra.Command` -- `provisioning` subcommand; lists clusters currently provisioning with details (duration, install retries, install logs)
- `NewDeprovisioningReportCommand() *cobra.Command` -- `deprovisioning` subcommand; lists clusters currently deprovisioning with details (duration, finalizers)
- `(o *ProvisioningReportOptions) Complete(cmd, args) error` -- Completes options (no-op)
- `(o *ProvisioningReportOptions) Validate(cmd) error` -- Validates options (no-op)
- `(o *ProvisioningReportOptions) Run(dynClient client.Client) error` -- Executes provisioning report
- `(o *DeprovisioningReportOptions) Complete(cmd, args) error` -- Completes options (no-op)
- `(o *DeprovisioningReportOptions) Validate(cmd) error` -- Validates options (no-op)
- `(o *DeprovisioningReportOptions) Run(dynClient client.Client) error` -- Executes deprovisioning report

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment types
- `github.com/openshift/hive/contrib/pkg/utils` -- Kube client creation
- `sigs.k8s.io/controller-runtime/pkg/client` -- Controller-runtime client for listing ClusterDeployments
- `github.com/spf13/cobra` -- CLI framework

## Capabilities

- List all clusters currently being provisioned with install log output, retry counts, and provisioning duration
- List all clusters currently being deprovisioned with finalizer details and deprovisioning duration
- Filter reports by cluster type label and creation age (less-than / greater-than duration)

## Understanding Score

0.9

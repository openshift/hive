# Module atlas

## Responsibility

Provides CLI commands for the `hiveutil` tool to deprovision (destroy) cloud resources for OpenShift clusters across multiple cloud providers. Includes both a generic metadata.json-based destroyer and legacy per-platform subcommands.

## Public Interface/API

**Types:**
- `AzureOptions` -- Options for Azure cluster deprovisioning (cloud name, resource group, base domain resource group)

**Functions:**
- `NewDeprovisionCommand() *cobra.Command` -- Top-level `deprovision` command; supports generic deprovisioning via `--metadata-json-secret-name` and adds per-platform subcommands
- `NewDeprovisionAWSWithTagsCommand() *cobra.Command` -- `aws-tag-deprovision` subcommand; destroys AWS resources by tag key=value pairs
- `NewDeprovisionAzureCommand(logLevel string) *cobra.Command` -- `azure` subcommand; destroys Azure resources by infra ID
- `NewDeprovisionGCPCommand(logLevel string) *cobra.Command` -- `gcp` subcommand; destroys GCP resources by infra ID and region
- `NewDeprovisionIBMCloudCommand(logLevel string) *cobra.Command` -- `ibmcloud` subcommand; destroys IBM Cloud resources by infra ID, region, base domain, and cluster name
- `NewDeprovisionNutanixCommand(logLevel string) *cobra.Command` -- `nutanix` subcommand; destroys Nutanix resources by infra ID
- `NewDeprovisionOpenStackCommand(logLevel string) *cobra.Command` -- `openstack` subcommand; destroys OpenStack resources by infra ID and cloud name
- `NewDeprovisionvSphereCommand(logLevel string) *cobra.Command` -- `vsphere` subcommand; destroys vSphere resources by infra ID

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` -- Kube client, logger, secret loading
- `github.com/openshift/hive/pkg/constants` -- Env var names, secret keys
- `github.com/openshift/hive/pkg/creds` -- Generic credential configuration dispatch
- `github.com/openshift/hive/pkg/creds/aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere` -- Per-cloud credential configuration
- `github.com/openshift/hive/pkg/gcpclient` -- GCP project ID extraction
- `github.com/openshift/hive/pkg/ibmclient` -- IBM Cloud client for CIS and account lookup
- `github.com/openshift/installer/pkg/destroy/aws`, `azure`, `gcp`, `ibmcloud`, `nutanix`, `openstack`, `vsphere` -- Installer destroy implementations
- `github.com/openshift/installer/pkg/destroy/providers` -- Destroyer registry
- `github.com/openshift/installer/pkg/types` -- Cluster metadata types
- `github.com/spf13/cobra` -- CLI framework

## Capabilities

- Generic cluster deprovisioning using metadata.json Secret and the installer's provider registry
- Legacy per-platform deprovisioning subcommands for AWS (tag-based), Azure, GCP, IBM Cloud, Nutanix, OpenStack, vSphere
- Credential loading from environment variables and Kubernetes secrets per cloud provider
- Configurable log levels

## Understanding Score

0.9

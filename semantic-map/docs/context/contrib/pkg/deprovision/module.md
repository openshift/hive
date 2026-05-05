# Module atlas

## Responsibility

Implements the `deprovision` command group for hiveutil: destroys cloud resources for a previously provisioned cluster. Provides a generic metadata.json-based destroy path and legacy per-cloud subcommands.

## Public Interface/API

- `NewDeprovisionCommand() *cobra.Command` — top-level `deprovision` command; primary path takes `metadata.json` and delegates to openshift-installer destroy
- `NewDeprovisionAWSWithTagsCommand() *cobra.Command` — legacy: destroys AWS resources by tag filter
- `NewDeprovisionAzureCommand() *cobra.Command` — legacy: destroys Azure resource group
- `NewDeprovisionGCPCommand() *cobra.Command` — legacy: destroys GCP resources
- `NewDeprovisionIBMCloudCommand() *cobra.Command` — legacy: destroys IBM Cloud resources
- `NewDeprovisionNutanixCommand() *cobra.Command` — destroys Nutanix resources
- `NewDeprovisionOpenStackCommand() *cobra.Command` — legacy: destroys OpenStack resources
- `NewDeprovisionvSphereCommand() *cobra.Command` — legacy: destroys vSphere resources
- `AzureOptions` — exported options struct for Azure deprovision

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/utils` — shared helpers
- `github.com/openshift/hive/pkg/constants` — platform constants
- `github.com/openshift/hive/pkg/creds` — generic credential loading
- `github.com/openshift/hive/pkg/creds/{aws,azure,gcp,ibmcloud,nutanix,openstack,vsphere}` — per-cloud credential loading
- `github.com/openshift/hive/pkg/gcpclient` — GCP client for service account parsing
- `github.com/openshift/hive/pkg/ibmclient` — IBM Cloud client
- `github.com/openshift/installer/pkg/destroy/{aws,azure,gcp,ibmcloud,nutanix,openstack,vsphere}` — openshift-installer destroy providers
- `github.com/openshift/installer/pkg/types` — ClusterMetadata, platform-specific metadata types

## Capabilities

- Generic destroy: reads `metadata.json` (infraID, platform metadata), loads credentials, delegates to openshift-installer destroy providers
- Legacy per-cloud subcommands for backwards compatibility (AWS tag-based, Azure resource group, GCP, IBM Cloud, Nutanix, OpenStack, vSphere)
- Supports credential loading from files or environment variables
- Covers all 7 cloud platforms supported by Hive

## Understanding Score

0.8

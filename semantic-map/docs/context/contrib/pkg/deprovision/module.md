<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/deprovision/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AzureOptions` — AzureOptions is the set of options to deprovision an Azure cluster
- `NewDeprovisionAWSWithTagsCommand` — NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
- `NewDeprovisionAzureCommand` — NewDeprovisionAzureCommand is the entrypoint to create the azure deprovision subcommand
- `NewDeprovisionCommand` — NewDeprovisionCommand is the entrypoint to create the 'deprovision' subcommand
- `NewDeprovisionGCPCommand` — NewDeprovisionGCPCommand is the entrypoint to create the GCP deprovision subcommand
- `NewDeprovisionIBMCloudCommand` — NewDeprovisionIBMCloudCommand is the entrypoint to create the IBM Cloud deprovision subcommand
- `NewDeprovisionNutanixCommand`
- `NewDeprovisionOpenStackCommand` — NewDeprovisionOpenStackCommand is the entrypoint to create the OpenStack deprovision subcommand
- `NewDeprovisionvSphereCommand` — NewDeprovisionvSphereCommand is the entrypoint to create the vSphere deprovision subcommand

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/creds`
- `github.com/openshift/hive/pkg/creds/aws`
- `github.com/openshift/hive/pkg/creds/azure`
- `github.com/openshift/hive/pkg/creds/gcp`
- `github.com/openshift/hive/pkg/creds/ibmcloud`
- `github.com/openshift/hive/pkg/creds/nutanix`
- `github.com/openshift/hive/pkg/creds/openstack`
- `github.com/openshift/hive/pkg/creds/vsphere`
- `github.com/openshift/hive/pkg/gcpclient`
- `github.com/openshift/hive/pkg/ibmclient`
- `github.com/openshift/installer/pkg/destroy/aws`
- `github.com/openshift/installer/pkg/destroy/azure`
- `github.com/openshift/installer/pkg/destroy/gcp`
- `github.com/openshift/installer/pkg/destroy/ibmcloud`
- `github.com/openshift/installer/pkg/destroy/nutanix`
- `github.com/openshift/installer/pkg/destroy/openstack`
- `github.com/openshift/installer/pkg/destroy/providers`
- `github.com/openshift/installer/pkg/destroy/vsphere`
- `github.com/openshift/installer/pkg/types`
- `github.com/openshift/installer/pkg/types/aws`
- `github.com/openshift/installer/pkg/types/azure`
- `github.com/openshift/installer/pkg/types/gcp`
- `github.com/openshift/installer/pkg/types/ibmcloud`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/openshift/installer/pkg/types/openstack`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `log`
- `os`
- `strings`

## Capabilities

- **`package`** name(s): **deprovision**.
- Go **`import`** edges listed below (37 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/deprovision`.

## Understanding Score

0.0

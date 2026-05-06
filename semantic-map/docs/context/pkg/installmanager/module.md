# Module atlas

## Responsibility

Coordinates executing the `openshift-install` binary inside a provisioning pod: runs install phases, modifies generated assets (install-config, manifests, credentials), uploads artifacts (admin kubeconfig, password, metadata) back to the Kubernetes API, cleans up DNS on failure, and optionally uploads installer logs to S3.

## Public Interface/API

- `InstallManager` struct -- main orchestrator with exported fields: LogLevel, WorkDir, LogsDir, ClusterID, ClusterName, ClusterProvisionName, ClusterProvision, Namespace, InstallConfigMountPath, PullSecretMountPath, ManifestsMountPath, DynamicClient
  - `Complete(args []string) error` -- set fields from command args
  - `Validate() error` -- validate options
  - `Run() error` -- entrypoint to start the install process
- `NewInstallManagerCommand() *cobra.Command` -- creates the `install-manager` subcommand for hiveutil
- `LogUploaderActuator` interface -- `IsConfigured() bool`, `UploadLogs(clusterName, clusterprovision, client, log, filenames...) error`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient`, `pkg/ibmclient`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/dnszone`, `pkg/controller/machinepool`, `pkg/controller/utils`
- `github.com/openshift/hive/pkg/creds`, `pkg/creds/azure`, `pkg/creds/gcp`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/labels`, `pkg/util/scheme`, `pkg/util/yaml`
- `github.com/openshift/installer/pkg/destroy/{aws,azure,gcp,ibmcloud,nutanix,openstack,vsphere,providers}`
- `github.com/openshift/installer/pkg/types` and platform-specific sub-packages
- `github.com/spf13/cobra`, `github.com/tidwall/gjson`, `github.com/tidwall/sjson`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- Runs `openshift-install create cluster` and monitors for completion
- Reads and uploads cluster metadata, admin kubeconfig, and admin password to Kubernetes secrets
- Modifies install-config with credential overrides and additional manifests
- Multi-platform cleanup on failed provisions using openshift-installer destroy libraries
- DNS zone cleanup (AWS, Azure, GCP) on install failures when ManageDNS is enabled
- S3 log upload for failed provisions via LogUploaderActuator
- Fake install mode for testing (skips actual cluster provisioning)

## Understanding Score

0.85

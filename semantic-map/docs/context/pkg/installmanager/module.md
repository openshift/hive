# Module: pkg/installmanager

## Responsibility

Orchestrates the full lifecycle of an OpenShift cluster installation from inside the installer pod. This is the `hiveutil install-manager` cobra command that runs as the main container in provisioning Jobs. It coordinates: loading credentials and secrets, copying installer binaries, preparing the install-config, running `openshift-install create manifests`, applying customizations (manifest patches from ClusterDeploymentCustomization), running `openshift-install create ignition-configs`, running `openshift-install create cluster`, uploading admin kubeconfig/password secrets, updating ClusterProvision status with metadata/infraID/installLog, gathering logs on failure (bootstrap SSH + must-gather), uploading logs to S3, and cleaning up failed provisions via platform-specific destroyers.

## Public Interface/API

- `NewInstallManagerCommand() *cobra.Command` -- entrypoint for `hiveutil install-manager NAMESPACE CLUSTER_PROVISION_NAME`.
- `InstallManager` -- main orchestration struct with fields for work/logs directories, cluster metadata, and function pointers for testability:
  - `Complete(args []string) error` -- wires up function pointers (loadSecrets, provisionCluster, readClusterMetadata, etc.), initializes logger with random installID, validates workdir. Swaps to fake implementations when `FAKE_CLUSTER_INSTALL` env var is set.
  - `Validate() error` -- currently a no-op.
  - `Run() error` -- the main workflow: loads ClusterProvision and ClusterDeployment, loads secrets/creds, sets up SSH agent, copies binaries, generates assets, reads cluster metadata, uploads kubeconfig/password, waits for ClusterProvision to transition to Provisioning stage (via watch), calls `openshift-install create cluster`, handles failures (pause, gather logs, upload logs), updates install log on ClusterProvision.
- `LogUploaderActuator` -- interface for cloud-specific log upload (`IsConfigured()`, `UploadLogs(...)`). Currently only `s3LogUploaderActuator` is implemented.

## Key Internal Workflows

1. **Secret Loading** (`loadSecrets`): Configures platform credentials via `pkg/creds`, projects install-config, pull secret, manifests, SSH keys, and bound SA signing key from Secrets/ConfigMaps to filesystem paths.
2. **Asset Generation** (`generateAssets`): Runs `openshift-install create manifests`, patches worker MachineSet manifests (extra security group for AWS, machine pool labels), copies user-provided manifests, overrides credentials mode, applies ClusterDeploymentCustomization patches, then runs `openshift-install create ignition-configs`.
3. **Failed Provision Cleanup** (`cleanupFailedProvision`): Uses openshift-installer destroyer libraries per-platform (AWS, Azure, GCP, OpenStack, vSphere, IBM Cloud, Nutanix) to tear down partially-provisioned infrastructure, then cleans up DNS zones.
4. **DNS Cleanup** (`dnscleanup.go`): Cleans stale DNS record sets for managed DNS zones on AWS, Azure, and GCP using respective cloud clients.
5. **Metadata Scrubbing** (`scrubMetadataJSON`): Streaming JSON rewriter that redacts `username` and `password` fields from metadata.json before storing on ClusterProvision.
6. **Fake Install Mode** (`fake.go`): Provides `fakeProvisionCluster`, `fakeReadClusterMetadata`, `fakeLoadAdminPassword` for testing/development.

## Internal Dependencies

Key Hive-internal imports:
- `apis/hive/v1` -- ClusterProvision, ClusterDeployment, MachinePool, DNSZone
- `pkg/awsclient`, `pkg/azureclient`, `pkg/gcpclient`, `pkg/ibmclient` -- cloud clients for cleanup/log upload
- `pkg/constants` -- mount paths, env var names, platform constants
- `pkg/controller/dnszone` -- DNS record set deletion
- `pkg/controller/machinepool` -- VPC ID lookup for extra security groups
- `pkg/controller/utils` -- log fields, manifest patches, stateful set calculations
- `pkg/creds`, `pkg/creds/azure`, `pkg/creds/gcp` -- credential configuration per platform
- `pkg/resource` -- `DeleteAnyExistingObject`
- `pkg/util/labels`, `pkg/util/scheme`, `pkg/util/yaml` -- label helpers, scheme, YAML patching
- `contrib/pkg/utils` -- secret/configmap loading, cert bundle building

Key external:
- `github.com/openshift/installer/pkg/destroy/*` -- platform-specific destroyers (7 platforms)
- `github.com/openshift/installer/pkg/types/*` -- ClusterMetadata, InstallConfig, platform metadata types
- `github.com/tidwall/gjson`, `github.com/tidwall/sjson` -- JSON path querying/setting for manifest patching
- `github.com/json-iterator/go` -- streaming JSON for metadata scrubbing

## Capabilities

- **Package**: `installmanager`
- Source files: `installmanager.go` (main, ~2287 lines), `dnscleanup.go`, `loguploaderactuator.go`, `s3loguploaderactuator.go`, `fake.go`.
- 75 unique import paths.
- Supports all 7 Hive platforms for cleanup plus BareMetal for SSH key handling.

## Understanding Score

0.85

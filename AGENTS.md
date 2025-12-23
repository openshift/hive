# AGENTS.md

Instructions for AI agents working on the OpenShift/Hive project.

## Project Overview

OpenShift Hive is an operator which runs as a service on top of Kubernetes/OpenShift.
The Hive service can be used to provision and perform initial configuration of OpenShift clusters.

Hive uses the [OpenShift installer](https://github.com/openshift/installer) for cluster provisioning.

### Supported Cloud Providers
- AWS
- Azure
- Google Cloud Platform (GCP)
- IBM Cloud
- Nutanix
- OpenStack
- vSphere
- Bare Metal

## Common development commands

The project uses `make` for automation. All commands should be run from the repository root.

### Development
```bash
make update # updates generated code
make build # compiles the project binaries
make clean # cleans up build artifacts
make all # runs vendor, update, test, and build in sequence

# Dependency management
make vendor # updates vendor directory
make modcheck # checks for dependency mismatches between root and submodules
make modfix # fixes dependency mismatches automatically
```

### Testing
```bash
make test # runs unit tests (excludes e2e tests)
make verify # verifies generated code and formatting
```

**Note**: `make test` runs unit tests for `./pkg/...`, `./cmd/...`, `./contrib/...`, and submodules. E2E tests are excluded and must be run separately using `make test-e2e`, `make test-e2e-pool`, etc.

## Architecture

### File Structure

- **`apis/`**: API definitions (separate Go submodule)
  - `hive/v1/`: Hive v1 APIs (ClusterDeployment, SyncSet, etc.)
  - `hiveinternal/v1alpha1/`: Internal APIs
  - `hivecontracts/v1alpha1/`: Contract APIs
- **`cmd/`**: Binary entry points
- **`pkg/`**: Package source code
- **`config/`**: Kubernetes YAML manifests for deploying the operator
- **`docs/`**: Developer and user documentation
- **`hack/`**: Developer tools and scripts
- **`test/e2e/`**: End-to-end tests

#### Entry Points (`cmd/`)
- **`cmd/manager/`**: Main entry point for Hive controllers
- **`cmd/operator/`**: Main entry point for Hive operator (deploys and manages other components)
- **`cmd/hiveadmission/`**: Admission webhook server for CR validation

#### Package Source Code (`pkg/`)
- **`pkg/controller/`**: Operator controllers (see Controllers section below)
- **`pkg/install/`**: Installation logic and OpenShift installer integration
- **`pkg/installmanager/`**: Manages cluster installation process (runs openshift-install)
- **`pkg/operator/`**: Hive operator logic for deploying and managing components
- **`pkg/clusterresource/`**: Builds cluster resources and install configs
- **`pkg/imageset/`**: Manages ClusterImageSet and installer image resolution
- **`pkg/creds/`**: Manages cloud provider credentials for cluster provisioning
- **`pkg/resource/`**: Utilities for applying resources to remote clusters
- **`pkg/remoteclient/`**: Client for connecting to and managing remote clusters
- **`pkg/{awsclient,azureclient,gcpclient,ibmclient}/`**: Cloud provider-specific client implementations
- **`pkg/manageddns/`**: Managed DNS functionality
- **`pkg/util/`**: Utility functions
- **`pkg/version/`**: Logic for operator version

#### Operator Controllers (`pkg/controller/`)
- **`pkg/controller/argocdregister`**: Ensures provisioned clusters are added to the ArgoCD cluster registry
- **`pkg/controller/awsprivatelink`**: Manages AWS PrivateLink configurations for clusters
- **`pkg/controller/clusterclaim`**: Manages ClusterClaim resources for requesting clusters from pools
- **`pkg/controller/clusterdeployment`**: Core controller that reconciles ClusterDeployments, orchestrating cluster provisioning and lifecycle
- **`pkg/controller/clusterdeprovision`**: Handles cluster deprovisioning and cleanup
- **`pkg/controller/clusterpool`**: Manages ClusterPools for pre-provisioning clusters
- **`pkg/controller/clusterpoolnamespace`**: Manages namespaces for cluster pools
- **`pkg/controller/clusterprovision`**: Manages ClusterProvision resources and install jobs
- **`pkg/controller/clusterrelocate`**: Handles cluster relocation between Hive instances
- **`pkg/controller/clusterstate`**: Syncs cluster state from remote clusters to ClusterDeployment status
- **`pkg/controller/clustersync`**: Applies SyncSets and SelectorSyncSets to provisioned clusters
- **`pkg/controller/clusterversion`**: Manages cluster version updates
- **`pkg/controller/controlplanecerts`**: Manages control plane certificates
- **`pkg/controller/dnsendpoint`**: Manages DNS endpoints for clusters
- **`pkg/controller/dnszone`**: Manages DNS zones for cluster domains
- **`pkg/controller/fakeclusterinstall`**: Manages FakeClusterInstall resources for testing agent-based installations
- **`pkg/controller/hibernation`**: Handles cluster hibernation and resumption
- **`pkg/controller/machinepool`**: Manages MachinePools for worker nodes
- **`pkg/controller/metrics`**: Calculates and publishes Prometheus metrics
- **`pkg/controller/privatelink`**: Manages PrivateLink configurations acr
oss cloud providers
- **`pkg/controller/remoteingress`**: Manages ingress configurations for remote clusters
- **`pkg/controller/syncidentityprovider`**: Syncs identity providers to provisioned clusters
- **`pkg/controller/unreachable`**: Handles unreachable cluster scenarios
- **`pkg/controller/velerobackup`**: Manages Velero backups for clusters

## Git Commit Instructions

All commits should follow a standard format to ensure clarity and traceability.

- Title format: `<Subsystem>: <Title>`
- Include a footer annotation when AI tools were used to generate or significantly assist

### Example
```text
HIVE-2980: How to refresh ClusterPool cloud creds
Add doc content describing different ways to rotate a ClusterPool's
cloud credentials.

Add a script, `hack/refresh-clusterpool-creds.sh` to nondisruptively
update the (currently AWS; other platforms TODO) cloud credentials for
all existing ClusterDeployments associated with a given ClusterPool.
- Accepts two args: the clusterpool namespace and name.
- Discovers the current AWS creds Secret from the clusterpool.
- Discovers all existing ClusterDeployments associated with the
  clusterpool.
- Discovers the AWS creds Secret for each CD.
- Patches that Secret with the `.data` of the clusterpool's Secret.

Assisted-by: <AI Model Name>
```

# Repository manifest

## High-level Purpose

OpenShift Hive is a Kubernetes operator that provides API-driven provisioning, configuration, and lifecycle management of OpenShift 4 clusters across multiple cloud providers (AWS, Azure, GCP, IBM Cloud, Nutanix, OpenStack, vSphere, bare metal), using the OpenShift installer under the hood.

## Tech Stack

- **Language:** Go (module `github.com/openshift/hive`, 128 packages)
- **Framework:** Kubernetes controller-runtime (operator pattern with reconcile loops)
- **APIs:** Custom Resource Definitions under `apis/hive/v1`, `apis/hivecontracts/v1alpha1`, `apis/hiveinternal/v1alpha1`
- **Cloud SDKs:** AWS SDK, Azure SDK, GCP client libraries, IBM Cloud SDK (each with mock wrappers)
- **Build:** Makefile, Dockerfile, Tekton pipelines (`.tekton/`)
- **Admission:** Validating webhooks via a standalone admission server
- **Scripting:** 25 shell scripts (hack/, CI), 4 Python scripts (hack/ utilities)
- **Dependencies:** 217 distinct external Go imports, 491 intra-module edges, 451 stdlib edges

## Directory Map

- **apis/** -- CRD type definitions and API group registrations
  - **hive/v1/** -- core types: ClusterDeployment, ClusterPool, ClusterClaim, MachinePool, DNSZone, SyncSet, HiveConfig, etc.
  - **hive/v1/{aws,azure,gcp,ibmcloud,nutanix,openstack,vsphere,baremetal,agent}/** -- per-platform type extensions
  - **hivecontracts/v1alpha1/** -- ClusterInstall contract interface for pluggable installers
  - **hiveinternal/v1alpha1/** -- internal types: ClusterSync, ClusterSyncLease, FakeClusterInstall
  - **helpers/** -- naming utilities for API objects
  - **scheme/** -- scheme registration aggregator
- **cmd/** -- binary entry points
  - **manager/** -- main controller-manager; starts all reconciliation controllers
  - **operator/** -- Hive operator; manages the Hive deployment itself (self-management)
  - **hiveadmission/** -- validating-webhook admission server
  - **util/** -- shared leader-election helper for cmd binaries
- **config/** -- Kubernetes manifests for deployment
  - **crds/** -- generated CRD YAML (21 CRDs)
  - **controllers/** -- controller deployment, RBAC, service
  - **hiveadmission/** -- webhook configs, RBAC, deployment
  - **operator/** -- operator deployment, RBAC
  - **rbac/** -- admin/reader/frontend role definitions
  - **prometheus/** -- monitoring deployment manifests
  - **samples/** -- example CR instances
  - **templates/** -- cluster deployment templates
- **contrib/** -- supplementary CLI tools and helpers
  - **cmd/hiveutil/** -- user-facing CLI for cluster creation, deprovision, pool management
  - **cmd/waitforjob/** -- utility to wait for Kubernetes Job completion
  - **pkg/** -- hiveutil subcommands: createcluster, deprovision, clusterpool, certificate, report, etc.
- **pkg/** -- core library code
  - **controller/** -- reconciliation controllers (one sub-package per controller)
    - **clusterdeployment/** -- core cluster lifecycle (provision, install, delete)
    - **clusterpool/** -- pool sizing, assignment, and lifecycle
    - **clusterclaim/** -- claim fulfillment from pools
    - **clusterprovision/** -- install-job orchestration and log monitoring
    - **clusterdeprovision/** -- teardown orchestration
    - **hibernation/** -- start/stop clusters with per-cloud actuators
    - **machinepool/** -- MachineSet generation per cloud platform
    - **clustersync/** -- SyncSet/SelectorSyncSet application to remote clusters
    - **dnszone/** -- DNS zone management with AWS/Azure/GCP actuators
    - **dnsendpoint/** -- external-DNS endpoint scraping
    - **privatelink/** -- AWS/GCP PrivateLink endpoint management
    - **awsprivatelink/** -- legacy AWS PrivateLink controller
    - **clusterstate/** -- periodic cluster health checks
    - **clusterversion/** -- version tracking from remote cluster
    - **controlplanecerts/** -- control-plane TLS certificate management
    - **remoteingress/** -- ingress certificate management on remote clusters
    - **syncidentityprovider/** -- identity provider synchronization
    - **velerobackup/** -- Velero backup integration
    - **clusterrelocate/** -- cluster migration between Hive instances
    - **clusterpoolnamespace/** -- namespace lifecycle for pool clusters
    - **argocdregister/** -- ArgoCD cluster registration
    - **unreachable/** -- unreachable-cluster detection
    - **fakeclusterinstall/** -- test-only fake installer
    - **metrics/** -- Prometheus metric collectors and custom dynamic-label metrics
    - **images/** -- controller image reference management
    - **utils/** -- shared controller helpers (conditions, expectations, client wrappers, rate-limited handlers)
  - **{awsclient,azureclient,gcpclient,ibmclient}/** -- cloud API client interfaces with mock/ subpackages
  - **clusterresource/** -- install-config and cluster resource generation per platform
  - **constants/** -- shared constants (labels, annotations, well-known names)
  - **creds/** -- credential extraction per cloud provider
  - **imageset/** -- ClusterImageSet release-image resolution
  - **install/** -- install-job manifest generation
  - **installmanager/** -- in-pod install orchestration (runs inside provision jobs)
  - **manageddns/** -- managed DNS domain helpers
  - **operator/** -- Hive operator reconciliation (self-deploy controllers, admission, assets)
  - **remoteclient/** -- kubeconfig-based client construction for spoke clusters
  - **resource/** -- kubectl-style apply/patch/delete helpers
  - **test/** -- builder-pattern test fixtures for all major CRD types
  - **util/** -- general utilities (contracts, labels, logrus adapters, scheme, YAML)
  - **validating-webhooks/** -- webhook handlers for admission validation
  - **version/** -- build-time version injection
- **hack/** -- development and CI scripts (codegen, e2e helpers, bundle generation)
- **test/** -- end-to-end and OTE (operator test environment) test suites
- **docs/** -- user and developer documentation, enhancement proposals
- **.tekton/** -- Tekton pipeline definitions for CI (MCE branch variants)
- **build/** -- import-rule verification

## Entry Points

| Binary | Package | Role |
|--------|---------|------|
| hive-controller-manager | `cmd/manager` | Starts all reconciliation controllers via controller-runtime manager |
| hive-operator | `cmd/operator` | Self-manages Hive deployment (installs/upgrades controllers, admission, CRDs) |
| hiveadmission | `cmd/hiveadmission` | Validating webhook server for CRD admission |
| hiveutil | `contrib/cmd/hiveutil` | CLI for cluster creation, deprovision, pool ops, diagnostics |
| waitforjob | `contrib/cmd/waitforjob` | Blocks until a Kubernetes Job completes |

### Reconciliation paths

The controller manager (`cmd/manager`) registers ~20 controllers. Each watches one or more CRDs and reconciles desired state:

1. **Cluster lifecycle:** ClusterDeployment controller drives the full provision-install-configure-deprovision cycle, delegating to ClusterProvision (job creation/monitoring) and ClusterDeprovision (teardown).
2. **Pool management:** ClusterPool controller maintains pool size; ClusterClaim controller assigns clusters from pools; ClusterPoolNamespace manages per-cluster namespaces.
3. **Day-2 operations:** ClusterSync applies SyncSets to remote clusters; MachinePool generates MachineSets; Hibernation starts/stops clusters; ClusterVersion/ClusterState track remote cluster health.
4. **Infrastructure:** DNSZone and DNSEndpoint manage DNS records; PrivateLink manages cloud private-connectivity endpoints; ControlPlaneCerts and RemoteIngress manage certificates.
5. **Operator self-management:** The Hive operator (`cmd/operator`) watches HiveConfig and reconciles the controller-manager deployment, admission webhooks, RBAC, and CRDs.

## Understanding Score

2.0 -- entry points identified, controller topology and reconciliation paths mapped; implementation-level flow tracing not yet performed.

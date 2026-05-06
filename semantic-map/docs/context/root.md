# Repository manifest

## High-level Purpose

OpenShift Hive is a Kubernetes operator that provides a declarative API to provision, configure, hibernate, pool, and deprovision OpenShift 4 clusters at scale across multiple cloud providers.

## Tech Stack

- **Language:** Go (128 packages)
- **Module:** `github.com/openshift/hive`
- **Framework:** controller-runtime / Kubernetes operator pattern
- **Cloud providers:** AWS, Azure, GCP, IBM Cloud, Nutanix, OpenStack, vSphere, Bare Metal
- **Installer:** delegates to `openshift-install` extracted from release images
- **API surface:** Kubernetes CRDs under `hive.openshift.io`, `hiveinternal.openshift.io`, `hivecontracts.openshift.io`
- **Build:** Makefile, Dockerfile, Tekton pipelines
- **CI tooling:** 25 shell scripts (hack/), 4 Python scripts
- **Dependencies:** 217 distinct external import paths, 491 same-module edges

## Directory Map

- **apis/** — CRD type definitions for Hive, HiveInternal, and HiveContracts API groups
  - **hive/v1/** — core types: ClusterDeployment, ClusterPool, MachinePool, SyncSet, DNSZone, etc.
  - **hivecontracts/v1alpha1/** — ClusterInstall contract interface for pluggable installers
  - **hiveinternal/v1alpha1/** — internal types: ClusterSync, ClusterSyncLease, FakeClusterInstall
  - **helpers/** — naming utility functions shared across API consumers
  - **scheme/** — aggregated scheme registration for all API groups
- **cmd/** — main packages producing operator and admission binaries
  - **hiveadmission/** — webhook validation server entry point
  - **manager/** — hive-controllers manager entry point
  - **operator/** — hive-operator deployer entry point
  - **util/** — shared leader-election helper for cmd binaries
- **config/** — Kubernetes manifests: CRDs, RBAC, deployments, webhooks, Prometheus, samples
- **contrib/** — developer CLI (hiveutil) and helper commands
  - **cmd/hiveutil/** — CLI for cluster creation, deprovision, diagnostics, and pool management
  - **cmd/waitforjob/** — job-completion watcher used by CI
  - **pkg/** — libraries backing hiveutil subcommands (createcluster, deprovision, report, etc.)
- **docs/** — user and developer documentation, architecture diagrams, enhancement proposals
- **hack/** — shell and Python scripts for CI, codegen, dev tooling, and scale testing
- **build/** — import verification rules for lint enforcement
- **overlays/** — Kustomize overlay templates for deployment variants
- **pkg/** — core library packages imported across all binaries
  - **controller/** — reconciliation controllers for every Hive CRD (20+ controllers)
  - **controller/utils/** — shared controller utilities: conditions, expectations, client wrappers
  - **controller/metrics/** — Prometheus metric definitions and custom collectors
  - **controller/privatelink/** — AWS and GCP PrivateLink actuators for private cluster connectivity
  - **operator/** — operator-level reconciler that deploys and manages Hive components
  - **awsclient/, azureclient/, gcpclient/, ibmclient/** — cloud API client wrappers with mock interfaces
  - **clusterresource/** — install-config builder for each cloud provider
  - **constants/** — shared constants used across controllers
  - **creds/** — per-provider credential extraction from Secrets
  - **imageset/** — release image resolution and installer image extraction
  - **install/** — install job and pod spec generation
  - **installmanager/** — sidecar logic that runs openshift-install and uploads artifacts
  - **manageddns/** — DNS domain management for Hive-managed zones
  - **remoteclient/** — kubeconfig-based client to managed clusters
  - **resource/** — kubectl-style apply/patch/delete helpers
  - **test/** — builder-pattern test fixtures for every CRD type
  - **util/** — low-level helpers: contracts, labels, logrus adapters, YAML utilities
  - **validating-webhooks/** — admission webhook handlers for CRD validation
  - **version/** — build-time version info
- **test/** — end-to-end and OTE (operator test extension) suites
  - **e2e/** — post-deploy, post-install, and destroy lifecycle tests
  - **ote/** — operator-test-extension framework for multi-provider e2e
- **.ai/** — AI coding assistant instructions
- **.tekton/** — Tekton pipeline definitions for CI build and PR checks
- **semantic-map/** — semantic-map artifact bundle (this context tree)

## Entry Points

- **`cmd/operator/main.go`** — Hive Operator: deploys and manages all other Hive components, reconciles HiveConfig CR
- **`cmd/manager/main.go`** — Hive Controllers: runs 20+ reconciliation controllers for all Hive CRDs
- **`cmd/hiveadmission/main.go`** — Admission Webhooks: validates CRD create/update requests
- **`contrib/cmd/hiveutil/main.go`** — hiveutil CLI: developer tool for cluster lifecycle operations
- **`contrib/cmd/waitforjob/main.go`** — job watcher: CI utility that blocks until a Job completes
- **`Makefile`** — primary build entry: compile, test, generate, verify targets

## Understanding Score

1.0

_Structural layout and entry points mapped from deterministic analysis; internal reconciliation logic not yet audited._

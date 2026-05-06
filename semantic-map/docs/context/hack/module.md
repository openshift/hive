# Module atlas

## Responsibility

Contains developer tooling scripts (shell, Python) and one Go utility for building, testing, deploying, and maintaining the Hive project. This directory is not a library -- it provides executable helpers for CI, development, and operational tasks.

## Public Interface/API

This directory contains shell/Python scripts and one Go `main` package. No importable API.

**Scripts:**
- `app_sre_build_deploy.sh` -- Build and deploy for App-SRE CI pipeline
- `bundle-gen.py` -- Generate OLM operator bundle manifests
- `codecov.sh` -- Upload code coverage reports
- `create-kind-cluster.sh` -- Create a local Kind cluster for development
- `create-service-account-secrets.sh` -- Create service account secrets for testing
- `duplicate_cd.sh` -- Duplicate a ClusterDeployment for testing
- `e2e-common.sh` -- Shared functions for e2e test scripts
- `e2e-pool-test.sh` -- Run ClusterPool e2e tests
- `e2e-test.sh` -- Run full e2e test suite
- `get-kubeconfig.sh` -- Retrieve kubeconfig for a managed cluster
- `github.py` -- GitHub API helper for CI automation
- `hiveadmission-dev-cert.sh` -- Generate dev certificates for hiveadmission webhook
- `local-e2e-test.sh` -- Run e2e tests locally
- `logextractor.sh` -- Extract logs from install pods
- `modcheck.go` -- Go `main` program that verifies `apis/go.mod` is in sync with the root `go.mod` (require, exclude, replace directives); supports `-f` fix mode
- `refresh-clusterpool-creds.sh` -- Refresh credentials for cluster pools
- `run-hive-locally.sh` -- Run Hive controllers locally against a cluster
- `set-additional-ca.sh` -- Configure additional CA certificates
- `ubi-build-deps.sh` -- Install UBI build dependencies
- `update-codegen.sh` -- Regenerate deepcopy, client, and informer code
- `verify-crd.sh` -- Verify CRD manifests are up to date
- `version2.sh` -- Version helper script

**Subdirectories:** `app-sre/`, `awsprivatelink/`, `gcpprivateserviceconnect/`, `grafana-dashboards/`, `hermetic/`, `regexes/`, `scaletest/`

## Internal Dependencies

- `golang.org/x/mod/modfile` -- Used by `modcheck.go` to parse and compare go.mod files

## Capabilities

- Verify `apis/go.mod` stays in sync with root `go.mod` (and optionally fix drift)
- Build and deploy Hive for App-SRE CI
- Generate OLM bundles
- Create and manage Kind clusters for local development
- Run e2e test suites (full and pool-specific)
- Extract install logs, retrieve kubeconfigs, duplicate ClusterDeployments
- Regenerate code (deepcopy, clients, informers) and verify CRDs
- Run Hive controllers locally for development

## Understanding Score

0.85

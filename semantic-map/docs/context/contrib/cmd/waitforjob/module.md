# Module atlas

## Responsibility

Binary entry point that watches a Kubernetes Job (install or uninstall) for a given ClusterDeployment and blocks until completion, failure, or timeout.

## Public Interface/API

This is a `main` package — no exported identifiers. The binary provides:

- CLI: `waitforjob CLUSTERNAME JOBTYPE` where JOBTYPE is `install` or `uninstall`
- Flags: `--log-level`, `--job-existence-timeout` (default 5m), `--job-execution-timeout` (default 90m)

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` — label constants for job matching (`ClusterDeploymentNameLabel`, `InstallJobLabel`, `UninstallJobLabel`)
- `github.com/openshift/hive/pkg/controller/utils` — `IsFailed`, `IsSuccessful` job status helpers
- `k8s.io/client-go` — clientset, watch, cache, clientcmd (external)
- `k8s.io/api/batch/v1` — Job type (external)

## Capabilities

- Watches for a labeled Job matching a ClusterDeployment name and job type
- Uses `UntilWithSync` watch with configurable existence and execution timeouts
- For uninstall jobs, tolerates the job not existing (may have been deleted already)
- Exits 0 on success, fatal on failure or timeout

## Understanding Score

0.9

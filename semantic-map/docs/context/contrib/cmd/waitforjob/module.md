# Module atlas

## Responsibility

CLI utility that watches for a Kubernetes batch Job (install or uninstall) associated with a Hive ClusterDeployment and blocks until the job succeeds, fails, or times out.

## Public Interface/API

`main` package — no exported identifiers. Produces the `waitforjob` binary.

- `main()` — parses CLI args (cluster name + job type), creates the cobra command, and executes it
- Usage: `waitforjob CLUSTERNAME JOBTYPE [OPTIONS]` where JOBTYPE is `install` or `uninstall`

CLI flags: `--log-level`, `--job-existence-timeout` (default 5m), `--job-execution-timeout` (default 90m)

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` — label keys (`ClusterDeploymentNameLabel`, `InstallJobLabel`, `UninstallJobLabel`)
- `github.com/openshift/hive/pkg/controller/utils` — `IsFailed`, `IsSuccessful` job status helpers
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `k8s.io/api/batch/v1` — Job type
- `k8s.io/client-go/kubernetes` — Kubernetes client
- `k8s.io/client-go/tools/clientcmd` — kubeconfig loading
- `k8s.io/client-go/tools/watch`, `k8s.io/client-go/tools/cache` — watch utilities

## Capabilities

- Watches for install or uninstall Jobs by label selector on a given ClusterDeployment name
- Uses `UntilWithSync` watch to block until job completes, fails, or times out
- Supports separate timeouts for job existence (waiting for creation) and job execution
- For uninstall jobs, tolerates the job already being deleted (non-fatal timeout)
- Loads kubeconfig from default client rules (KUBECONFIG env or ~/.kube/config)

## Understanding Score

0.9

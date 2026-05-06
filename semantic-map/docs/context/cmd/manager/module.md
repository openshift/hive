# Module atlas

## Responsibility

Entry-point binary for the Hive controller manager. Wires up all 24 Hive reconciliation controllers into a single controller-runtime manager with leader election, health/readiness probes, pprof, and metrics serving.

## Public Interface/API

`main` package — no exported identifiers. Produces the `manager` binary.

- `main()` — enables pprof, builds the cobra root command, and executes it
- `newRootCommand()` — unexported; creates a cobra command that configures logging, builds a controller-runtime `Manager`, registers all enabled controllers, and starts the manager with optional leader election
- `newControllerManagerOptions()` — unexported; returns default options with all 24 controllers enabled

Registered controllers (via `controllerFuncs` map):
`clusterclaim`, `clusterdeployment`, `clusterdeprovision`, `clusterpoolnamespace`, `clusterprovision`, `clusterrelocate`, `clusterstate`, `clustersync`, `clusterversion`, `controlplanecerts`, `dnsendpoint`, `dnszone`, `fakeclusterinstall`, `metrics`, `remoteingress`, `machinepool`, `syncidentityprovider`, `unreachable`, `velerobackup`, `clusterpool`, `hibernation`, `privatelink`, `awsprivatelink`, `argocdregister`

CLI flags: `--log-level`, `--controllers`, `--disabled-controllers`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — controller name constants
- `github.com/openshift/hive/cmd/util` — leader election helper (`RunWithLeaderElection`)
- `github.com/openshift/hive/pkg/constants` — namespace env var
- `github.com/openshift/hive/pkg/controller/{argocdregister,awsprivatelink,clusterclaim,clusterdeployment,clusterdeprovision,clusterpool,clusterpoolnamespace,clusterprovision,clusterrelocate,clusterstate,clustersync,clusterversion,controlplanecerts,dnsendpoint,dnszone,fakeclusterinstall,hibernation,machinepool,metrics,privatelink,remoteingress,syncidentityprovider,unreachable,velerobackup}` — individual controller `Add` functions
- `github.com/openshift/hive/pkg/controller/utils` — `GetHiveNamespace`, `SetupAdditionalCA`
- `github.com/openshift/hive/pkg/util/logrus` — logrus-to-logr adapter
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/openshift/hive/pkg/version` — build version string
- `github.com/spf13/cobra`, `github.com/spf13/pflag` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `sigs.k8s.io/controller-runtime` — manager, signals, config, metrics server
- `k8s.io/klog` — klog integration and flushing

## Capabilities

- Starts all 24 Hive reconciliation controllers in a single process
- Supports selectively enabling/disabling controllers via CLI flags
- Runs leader election via `cmd/util.RunWithLeaderElection` (skippable with `HIVE_SKIP_LEADER_ELECTION` env var)
- Exposes `/healthz` and `/readyz` HTTP endpoints on port 8080
- Serves Prometheus metrics on port 2112
- Enables pprof debugging endpoint on localhost:6060
- Configures additional CA certificates at startup

## Understanding Score

0.9

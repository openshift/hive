# Module atlas

## Responsibility

Binary entry point for the Hive controller manager. Registers all 24 reconciliation controllers into a single controller-runtime manager, configures leader election, and starts the process.

## Public Interface/API

This is a `main` package — no exported identifiers. The binary exposes:

- CLI via cobra with flags: `--log-level`, `--controllers` (select subset), `--disabled-controllers` (exclude subset)
- Metrics endpoint on `:2112`
- Health/readiness probes on `:8080` (`/healthz`, `/readyz`)
- pprof debug endpoint on `localhost:6060`

### Registered controllers (24)

| Controller | Package |
|------------|---------|
| clusterclaim | `pkg/controller/clusterclaim` |
| clusterdeployment | `pkg/controller/clusterdeployment` |
| clusterdeprovision | `pkg/controller/clusterdeprovision` |
| clusterpool | `pkg/controller/clusterpool` |
| clusterpoolnamespace | `pkg/controller/clusterpoolnamespace` |
| clusterprovision | `pkg/controller/clusterprovision` |
| clusterrelocate | `pkg/controller/clusterrelocate` |
| clusterstate | `pkg/controller/clusterstate` |
| clustersync | `pkg/controller/clustersync` |
| clusterversion | `pkg/controller/clusterversion` |
| controlplanecerts | `pkg/controller/controlplanecerts` |
| dnsendpoint | `pkg/controller/dnsendpoint` |
| dnszone | `pkg/controller/dnszone` |
| fakeclusterinstall | `pkg/controller/fakeclusterinstall` |
| hibernation | `pkg/controller/hibernation` |
| machinepool | `pkg/controller/machinepool` |
| metrics | `pkg/controller/metrics` |
| privatelink | `pkg/controller/privatelink` |
| awsprivatelink | `pkg/controller/awsprivatelink` |
| remoteingress | `pkg/controller/remoteingress` |
| syncidentityprovider | `pkg/controller/syncidentityprovider` |
| unreachable | `pkg/controller/unreachable` |
| velerobackup | `pkg/controller/velerobackup` |
| argocdregister | `pkg/controller/argocdregister` |

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — CRD types and controller name constants
- `github.com/openshift/hive/cmd/util` — leader election helper
- `github.com/openshift/hive/pkg/constants` — namespace env var and defaults
- `github.com/openshift/hive/pkg/controller/*` (24 packages) — each controller's `Add` registration function
- `github.com/openshift/hive/pkg/controller/utils` — shared helpers (namespace lookup, additional CA setup)
- `github.com/openshift/hive/pkg/util/logrus` — logrus-to-logr adapter for controller-runtime
- `github.com/openshift/hive/pkg/util/scheme` — shared Kubernetes scheme
- `github.com/openshift/hive/pkg/version` — build-time version string
- `sigs.k8s.io/controller-runtime` — manager, config, signals, metrics server (external)
- `github.com/spf13/cobra`, `github.com/spf13/pflag` — CLI framework (external)
- `k8s.io/klog`, `github.com/sirupsen/logrus` — logging (external)

## Capabilities

- Starts all Hive reconciliation controllers in a single process via controller-runtime manager
- Supports selective controller enablement/disablement via CLI flags
- Runs leader election (skippable via `HIVE_SKIP_LEADER_ELECTION` env var)
- Exposes Prometheus metrics, health probes, and pprof profiling endpoints
- Handles deprecated controller name mappings (e.g. `remotemachineset` → `machinepool`)

## Understanding Score

0.85

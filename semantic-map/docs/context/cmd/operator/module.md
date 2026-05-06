# Module atlas

## Responsibility

Entry-point binary for the Hive Operator. Creates a controller-runtime manager, registers operator-level controllers via `pkg/operator.AddToOperator`, and runs with leader election to manage the lifecycle of the HiveConfig CRD and associated Hive components.

## Public Interface/API

`main` package — no exported identifiers. Produces the `hive-operator` binary.

- `main()` — builds the cobra root command and executes it
- `newRootCommand()` — unexported; creates a cobra command that configures logging, builds a controller-runtime `Manager`, registers operator controllers, and starts the manager with leader election

CLI flags: `--log-level`

## Internal Dependencies

- `github.com/openshift/hive/cmd/util` — leader election helper (`RunWithLeaderElection`)
- `github.com/openshift/hive/pkg/operator` — `AddToOperator` registers operator controllers
- `github.com/openshift/hive/pkg/operator/hive` — `HiveOperatorNamespaceEnvVar` constant
- `github.com/openshift/hive/pkg/util/logrus` — logrus-to-logr adapter
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/openshift/hive/pkg/version` — build version string
- `github.com/openshift/generic-admission-server/pkg/cmd` — imported for side effects
- `github.com/spf13/cobra`, `github.com/spf13/pflag` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `sigs.k8s.io/controller-runtime` — manager, signals, config, metrics server
- `k8s.io/klog` — klog integration

## Capabilities

- Runs the Hive operator process that manages HiveConfig reconciliation
- Requires `HIVE_OPERATOR_NS` env var (set via downward API) to determine its namespace
- Exposes `/healthz` and `/readyz` HTTP endpoints on port 8080
- Serves Prometheus metrics on port 2112
- Uses leader election with lock name `hive-operator-leader`

## Understanding Score

0.9

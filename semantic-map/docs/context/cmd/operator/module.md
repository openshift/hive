# Module atlas

## Responsibility

Binary entry point for the Hive operator. Watches `HiveConfig` CRs and reconciles the Hive deployment itself — installing/upgrading the controller manager, admission webhooks, CRDs, and RBAC.

## Public Interface/API

This is a `main` package — no exported identifiers. The binary exposes:

- CLI via cobra with flag: `--log-level`
- Metrics endpoint on `:2112`
- Health/readiness probes on `:8080` (`/healthz`, `/readyz`)
- Requires `HIVE_OPERATOR_NS` env var to identify its own namespace

## Internal Dependencies

- `github.com/openshift/hive/pkg/operator` — `AddToOperator` registers the operator controller(s)
- `github.com/openshift/hive/pkg/operator/hive` — `HiveOperatorNamespaceEnvVar` constant
- `github.com/openshift/hive/cmd/util` — leader election helper
- `github.com/openshift/hive/pkg/util/logrus` — logrus-to-logr adapter
- `github.com/openshift/hive/pkg/util/scheme` — shared Kubernetes scheme
- `github.com/openshift/hive/pkg/version` — build-time version string
- `github.com/openshift/generic-admission-server/pkg/cmd` — side-effect import (external)
- `sigs.k8s.io/controller-runtime` — manager, config, signals, metrics server (external)
- `github.com/spf13/cobra` — CLI framework (external)

## Capabilities

- Starts the Hive operator process that self-manages the Hive deployment
- Delegates controller registration to `pkg/operator.AddToOperator`
- Runs leader election using Lease-based locks via `cmd/util`
- Exposes Prometheus metrics and health probe endpoints

## Understanding Score

0.8

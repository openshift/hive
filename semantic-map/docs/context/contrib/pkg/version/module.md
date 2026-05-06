# Module atlas

## Responsibility

Provides a CLI command for the `hiveutil` tool to print the Hive binary version information.

## Public Interface/API

**Functions:**
- `NewVersionCommand() *cobra.Command` -- Creates the `version` command that prints Hive version info via `pkg/version.String()`

## Internal Dependencies

- `github.com/openshift/hive/pkg/version` -- Version string provider
- `github.com/spf13/cobra` -- CLI framework

## Capabilities

- Print Hive version information to the log

## Understanding Score

0.9

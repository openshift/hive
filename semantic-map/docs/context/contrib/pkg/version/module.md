# Module atlas

## Responsibility

Implements the `version` subcommand for hiveutil: displays the Hive build version string.

## Public Interface/API

- `NewVersionCommand() *cobra.Command` — prints `pkg/version.Version` to stdout

## Internal Dependencies

- `github.com/openshift/hive/pkg/version` — build-time version variable

## Capabilities

- Prints the compiled-in Hive version string

## Understanding Score

0.95

# Module atlas

## Responsibility

Implements the `test-resource` command for hiveutil: a developer tool that tests the resource helper's apply and patch operations against a live cluster.

## Public Interface/API

- `NewTestResourceCommand() *cobra.Command` — `test-resource` subcommand: reads a YAML/JSON file and performs apply + server-side patch via the resource helper

## Internal Dependencies

- `github.com/openshift/hive/pkg/resource` — resource.Helper for apply and patch operations

## Capabilities

- Reads a resource manifest from a file path argument
- Applies the resource to the cluster via `resource.Helper.Apply`
- Patches the resource via `resource.Helper.Patch` with `types.StrategicMergePatchType`
- Intended as a smoke-test tool for verifying resource helper behavior

## Understanding Score

0.85

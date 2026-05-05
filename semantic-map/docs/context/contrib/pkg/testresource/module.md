# Module atlas

## Responsibility

Implements the `test-resource` command for hiveutil: a developer tool that tests the resource helper's apply and patch operations against a live cluster.

## Public Interface/API

- `NewTestResourceCommand() *cobra.Command` — `resource` parent command with two subcommands: `apply` and `patch`
- `apply RESOURCEFILE` subcommand — reads a YAML/JSON file and applies it via `resource.Helper.Apply`
- `patch PATCHFILE` subcommand — reads a patch file and applies it via `resource.Helper.Patch` with configurable patch type (json, merge, strategic)

## Internal Dependencies

- `github.com/openshift/hive/pkg/resource` — resource.Helper for apply and patch operations

## Capabilities

- Provides two distinct subcommands for testing resource helper behavior
- `apply`: reads a resource manifest, obtains its info, and applies it to a target cluster specified via `--kubeconfig`
- `patch`: reads a patch file and applies it to a named resource with configurable patch type, kind, apiVersion, namespace, and name flags
- Both subcommands require an explicit `--kubeconfig` flag
- Intended as a smoke-test/developer tool for verifying resource helper behavior

## Understanding Score

0.9

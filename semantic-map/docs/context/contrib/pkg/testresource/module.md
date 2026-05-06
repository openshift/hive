# Module atlas

## Responsibility

Provides a CLI command for the `hiveutil` tool to test Hive's resource apply and patch functions against a target cluster, useful for development and debugging.

## Public Interface/API

**Functions:**
- `NewTestResourceCommand() *cobra.Command` -- Creates the `resource` command with `apply` and `patch` subcommands for testing Hive's resource helper

## Internal Dependencies

- `github.com/openshift/hive/pkg/resource` -- Hive resource helper (Apply, Patch, Info)
- `github.com/spf13/cobra` -- CLI framework
- `k8s.io/apimachinery/pkg/types` -- NamespacedName, PatchType

## Capabilities

- Test `resource.Apply` by reading a resource YAML file and applying it via the Hive resource helper
- Test `resource.Patch` by applying a patch file (JSON, merge, or strategic merge) to a named resource
- Requires a kubeconfig to connect to the target cluster

## Understanding Score

0.9

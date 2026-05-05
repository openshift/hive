# Module atlas

## Responsibility

Shared package-level variables used across the `awsprivatelink` subcommand packages to pass the initialized dynamic client and credentials Secret.

## Public Interface/API

- `var CredsSecret *corev1.Secret` — optional AWS credentials Secret loaded from `--creds-secret` flag
- `var DynamicClient client.Client` — controller-runtime client initialized in the parent command's `PersistentPreRun`

## Internal Dependencies

- `k8s.io/api/core/v1` — Secret type (external)
- `sigs.k8s.io/controller-runtime/pkg/client` — Client interface (external)

## Capabilities

- Provides a shared state mechanism between `awsprivatelink`, `enable`, `disable`, and `endpointvpc` commands
- Set once during `PersistentPreRun` of the parent command and read by all subcommands

## Understanding Score

0.95

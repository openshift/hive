# Module atlas

## Responsibility

Holds shared package-level variables used across the `contrib/pkg/awsprivatelink` subcommand tree, providing a common place for the dynamic client and optional credentials secret initialized during `PersistentPreRun`.

## Public Interface/API

**Vars:**
- `var CredsSecret *corev1.Secret` — optional AWS credentials secret loaded from `--creds-secret` flag
- `var DynamicClient client.Client` — controller-runtime client initialized during command pre-run

## Internal Dependencies

- `k8s.io/api/core/v1` — Secret type
- `sigs.k8s.io/controller-runtime/pkg/client` — Client interface

## Capabilities

- Provides shared mutable state (client and credentials secret) for the awsprivatelink command family
- Set by the parent command's `PersistentPreRun` and consumed by subcommands (enable, disable, endpointvpc)

## Understanding Score

0.9

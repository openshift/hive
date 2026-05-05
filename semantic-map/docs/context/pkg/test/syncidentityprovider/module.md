# Module atlas

## Responsibility

Test builder for `hivev1.SyncIdentityProvider` resources. Provides options for name, namespace, cluster deployment references, and identity provider entries.

## Public Interface/API

- `type Option func(*hivev1.SyncIdentityProvider)` -- functional option type
- `Build(opts ...Option) *hivev1.SyncIdentityProvider` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` / `WithNamespace(namespace) Option`
- `ForClusterDeployments(names...) Option` -- sets `Spec.ClusterDeploymentRefs` from a list of CD names
- `ForIdentities(names...) Option` -- sets `Spec.IdentityProviders` with named IdentityProvider entries

## Internal Dependencies

- `github.com/openshift/api/config/v1` -- IdentityProvider type
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `syncidentityprovider`
- Standard Hive test builder pattern

## Understanding Score

0.9

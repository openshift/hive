# Module atlas

## Responsibility

Builder-pattern test fixture for constructing `hivev1.SyncIdentityProvider` objects in unit tests. Provides options for associating cluster deployments and configuring identity providers.

## Public Interface/API

- `type Option func(*hivev1.SyncIdentityProvider)` -- functional option type
- `Build(opts ...Option) *hivev1.SyncIdentityProvider` -- constructs a SyncIdentityProvider from options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- returns a pre-configured builder
- `Generic(opt generic.Option) Option` -- adapts a generic.Option
- `WithName(name string) Option` -- sets object name
- `WithNamespace(namespace string) Option` -- sets object namespace
- `ForClusterDeployments(clusterDeploymentNames ...string) Option` -- sets ClusterDeploymentRefs
- `ForIdentities(names ...string) Option` -- sets IdentityProviders with given names

## Internal Dependencies

- `github.com/openshift/api/config/v1` -- IdentityProvider type
- `github.com/openshift/hive/apis/hive/v1` -- SyncIdentityProvider type
- `github.com/openshift/hive/pkg/test/generic` -- shared test builder primitives
- `k8s.io/api/core/v1` -- LocalObjectReference
- `k8s.io/apimachinery/pkg/runtime` -- ObjectTyper

## Capabilities

- Construct `hivev1.SyncIdentityProvider` test fixtures via functional options
- Associate identity providers with cluster deployments by name
- Configure OpenShift identity provider entries

## Understanding Score

0.85

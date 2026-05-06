# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterRelocate` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.ClusterRelocate)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterRelocate` -- constructs a ClusterRelocate from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, and name (no namespace -- cluster-scoped)
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithKubeconfigSecret(namespace, name string) Option` -- sets the KubeconfigSecretRef
- `WithClusterDeploymentSelector(key, value string) Option` -- sets ClusterDeploymentSelector with a MatchLabels entry

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.ClusterRelocate` test fixtures with kubeconfig secret reference and label selector
- FullBuilder omits namespace (cluster-scoped resource)
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

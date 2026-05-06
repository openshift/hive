# Module atlas

## Responsibility

Builder-pattern test fixture for constructing `corev1.Namespace` objects in unit tests. Provides functional options and a fluent Builder interface with support for generic object metadata options.

## Public Interface/API

- `type Option func(*corev1.Namespace)` -- functional option type
- `Build(opts ...Option) *corev1.Namespace` -- constructs a Namespace from options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(name string, typer runtime.ObjectTyper) Builder` -- returns a builder pre-configured with TypeMeta, ResourceVersion, and Name
- `Generic(opt generic.Option) Option` -- adapts a generic.Option to a namespace Option

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic` -- shared test builder primitives
- `k8s.io/api/core/v1` -- Namespace type
- `k8s.io/apimachinery/pkg/runtime` -- ObjectTyper

## Capabilities

- Construct `corev1.Namespace` test fixtures via functional options
- Compose generic metadata options (name, type meta, resource version) with namespace-specific options
- Fluent builder pattern for accumulating options across test setup

## Understanding Score

0.85

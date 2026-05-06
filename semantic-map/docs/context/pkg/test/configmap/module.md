# Module atlas

## Responsibility

Test-only builder utility for constructing `corev1.ConfigMap` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*corev1.ConfigMap)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *corev1.ConfigMap` -- constructs a ConfigMap from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithDataKeyValue(key string, value string) Option` -- adds a key-value pair to Data
- `WithName(name string) Option`
- `WithNamespace(namespace string) Option`

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `corev1.ConfigMap` test fixtures with data entries
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

# Module atlas

## Responsibility

Test builder for `corev1.ConfigMap` resources. Provides options for setting name, namespace, and data key-value pairs.

## Public Interface/API

- `type Option func(*corev1.ConfigMap)` -- functional option type
- `Build(opts ...Option) *corev1.ConfigMap` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` -- sets object name
- `WithNamespace(namespace) Option` -- sets object namespace
- `WithDataKeyValue(key, value) Option` -- adds a string key-value to the ConfigMap's Data map

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `configmap`
- Standard Hive test builder pattern; builds core Kubernetes types rather than Hive CRDs

## Understanding Score

0.9

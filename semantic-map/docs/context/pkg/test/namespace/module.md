# Module atlas

## Responsibility

Test builder for `corev1.Namespace` resources. Provides the standard builder pattern with no resource-specific options beyond what `generic` provides.

## Public Interface/API

- `type Option func(*corev1.Namespace)` -- functional option type
- `Build(opts ...Option) *corev1.Namespace` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(name, typer) Builder` -- note: cluster-scoped (no namespace param)
- `Generic(opt) Option` -- adapts a `generic.Option`

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `namespace`
- Standard Hive test builder pattern; minimal -- no resource-specific options

## Understanding Score

0.9

# Module atlas

## Responsibility

Test builder for `corev1.Secret` resources. Provides options for setting name, namespace, data key-value pairs (byte slices), and secret type.

## Public Interface/API

- `type Option func(*corev1.Secret)` -- functional option type
- `Build(opts ...Option) *corev1.Secret` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` -- sets object name
- `WithNamespace(namespace) Option` -- sets object namespace
- `WithDataKeyValue(key, value) Option` -- adds a `[]byte` key-value to the Secret's Data map
- `WithType(t) Option` -- sets the Secret's type (e.g., Opaque, TLS)

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `secret`
- Standard Hive test builder pattern; builds core Kubernetes Secret types

## Understanding Score

0.9

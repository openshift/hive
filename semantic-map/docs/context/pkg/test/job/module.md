# Module atlas

## Responsibility

Test builder for `batchv1.Job` resources. Provides a minimal set of options for setting name and namespace.

## Public Interface/API

- `type Option func(*batchv1.Job)` -- functional option type
- `Build(opts ...Option) *batchv1.Job` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` -- sets object name
- `WithNamespace(namespace) Option` -- sets object namespace

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/batch/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `job`
- Standard Hive test builder pattern; builds core Kubernetes Job types

## Understanding Score

0.9

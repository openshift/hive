# Module atlas

## Responsibility

Test-only builder utility for constructing `batchv1.Job` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*batchv1.Job)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *batchv1.Job` -- constructs a Job from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithName(name string) Option`
- `WithNamespace(namespace string) Option`

## Internal Dependencies

- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/batch/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `batchv1.Job` test fixtures with name and namespace
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

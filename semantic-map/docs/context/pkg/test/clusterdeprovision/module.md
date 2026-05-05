# Module atlas

## Responsibility

Test builder for `hivev1.ClusterDeprovision` resources. Provides options for setting name/namespace, completion status, and authentication failure conditions.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeprovision)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterDeprovision` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` -- sets object name
- `WithNamespace(namespace) Option` -- sets object namespace
- `Completed() Option` -- sets `Status.Completed = true`
- `WithAuthenticationFailure() Option` -- adds an AuthenticationFailure condition via `controllerutils.SetClusterDeprovisionCondition`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils` -- condition-setting utilities
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `clusterdeprovision`
- Standard Hive test builder pattern

## Understanding Score

0.9

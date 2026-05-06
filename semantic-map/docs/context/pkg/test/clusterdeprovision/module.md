# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterDeprovision` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeprovision)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterDeprovision` -- constructs a ClusterDeprovision from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithName(name string) Option`
- `WithNamespace(namespace string) Option`
- `Completed() Option` -- sets Status.Completed to true
- `WithAuthenticationFailure() Option` -- adds an AuthenticationFailure condition with ConditionTrue

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.ClusterDeprovision` test fixtures with completion status and authentication failure conditions
- Uses `controllerutils.SetClusterDeprovisionCondition` for condition management
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

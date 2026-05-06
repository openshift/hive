# Module atlas

## Responsibility

Builder-pattern test fixture for constructing `appsv1.StatefulSet` objects in unit tests. Provides options for replicas and current replicas status, and uses `hivev1.DeploymentName` for the name parameter in `FullBuilder`.

## Public Interface/API

- `type Option func(*appsv1.StatefulSet)` -- functional option type
- `Build(opts ...Option) *appsv1.StatefulSet` -- constructs a StatefulSet from options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace string, name hivev1.DeploymentName, typer runtime.ObjectTyper) Builder` -- returns a pre-configured builder
- `Generic(opt generic.Option) Option` -- adapts a generic.Option
- `WithName(name string) Option` -- sets object name
- `WithNamespace(namespace string) Option` -- sets object namespace
- `WithReplicas(replicas int32) Option` -- sets spec.Replicas
- `WithCurrentReplicas(currentReplicas int32) Option` -- sets status.CurrentReplicas

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- DeploymentName type
- `github.com/openshift/hive/pkg/test/generic` -- shared test builder primitives
- `k8s.io/api/apps/v1` -- StatefulSet type
- `k8s.io/apimachinery/pkg/runtime` -- ObjectTyper
- `k8s.io/utils/ptr` -- ptr.To helper

## Capabilities

- Construct `appsv1.StatefulSet` test fixtures via functional options
- Configure replica count and current replicas status
- Integrate hive DeploymentName type for naming

## Understanding Score

0.85

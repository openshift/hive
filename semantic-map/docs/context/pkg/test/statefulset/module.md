# Module atlas

## Responsibility

Test builder for `appsv1.StatefulSet` resources. Provides options for name, namespace, desired replicas, and current replicas. The `FullBuilder` accepts a `hivev1.DeploymentName` for the name parameter, tying it to Hive's deployment naming conventions.

## Public Interface/API

- `type Option func(*appsv1.StatefulSet)` -- functional option type
- `Build(opts ...Option) *appsv1.StatefulSet` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name DeploymentName, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` -- sets object name
- `WithNamespace(namespace) Option` -- sets object namespace
- `WithReplicas(replicas) Option` -- sets `Spec.Replicas`
- `WithCurrentReplicas(currentReplicas) Option` -- sets `Status.CurrentReplicas`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- DeploymentName type
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/apps/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`

## Capabilities

- **Package**: `statefulset`
- Standard Hive test builder pattern; builds core Kubernetes StatefulSet types

## Understanding Score

0.9

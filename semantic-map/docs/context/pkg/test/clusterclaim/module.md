# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterClaim` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.ClusterClaim)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterClaim` -- constructs a ClusterClaim from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- returns a builder pre-configured with TypeMeta, ResourceVersion, namespace, and name
- `Generic(opt generic.Option) Option` -- adapts a generic option for ClusterClaim
- `WithPool(poolName string) Option` -- sets Spec.ClusterPoolName
- `WithCluster(clusterName string) Option` -- sets Spec.Namespace (the claimed cluster)
- `WithSubjects(subjects []rbacv1.Subject) Option` -- sets Spec.Subjects
- `WithCondition(cond hivev1.ClusterClaimCondition) Option` -- adds or replaces a status condition
- `WithLifetime(lifetime time.Duration) Option` -- sets Spec.Lifetime

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/rbac/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.ClusterClaim` test fixtures with pool reference, claimed cluster, subjects, conditions, and lifetime
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

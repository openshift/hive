# Module atlas

## Responsibility

Test builder for `hivev1.ClusterClaim` resources. Provides functional options to construct ClusterClaim fixtures with pool references, cluster assignments, RBAC subjects, conditions, and lifetime settings.

## Public Interface/API

- `type Option func(*hivev1.ClusterClaim)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterClaim` -- constructs a ClusterClaim by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithPool(poolName) Option` -- sets `Spec.ClusterPoolName`
- `WithCluster(clusterName) Option` -- sets `Spec.Namespace` (the claimed cluster)
- `WithSubjects(subjects) Option` -- sets `Spec.Subjects` (RBAC subjects)
- `WithCondition(cond) Option` -- adds or replaces a ClusterClaimCondition
- `WithLifetime(duration) Option` -- sets `Spec.Lifetime`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/rbac/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `time`

## Capabilities

- **Package**: `clusterclaim`
- Standard Hive test builder pattern

## Understanding Score

0.9

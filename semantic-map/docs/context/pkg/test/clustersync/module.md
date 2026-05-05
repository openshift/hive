# Module atlas

## Responsibility

Test builder for `hiveinternalv1alpha1.ClusterSync` resources (from the hiveinternal API group). Provides options for SyncSet and SelectorSyncSet status entries, conditions, and first-success timestamps.

## Public Interface/API

- `type Option func(*hiveinternalv1alpha1.ClusterSync)` -- functional option type
- `Build(opts ...Option) *hiveinternalv1alpha1.ClusterSync` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithSyncSetStatus(syncStatus) Option` -- appends a SyncStatus to `Status.SyncSets`
- `WithSelectorSyncSetStatus(syncStatus) Option` -- appends a SyncStatus to `Status.SelectorSyncSets`
- `WithCondition(cond) Option` -- adds or replaces a ClusterSyncCondition
- `WithFirstSuccessTime(time) Option` -- sets `Status.FirstSuccessTime`
- `WithNoFirstSuccessTime() Option` -- clears `Status.FirstSuccessTime`

## Internal Dependencies

- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `time`

## Capabilities

- **Package**: `clusterSync` (note: Go package name uses camelCase)
- Standard Hive test builder pattern; builds internal API types rather than hive/v1 types

## Understanding Score

0.9

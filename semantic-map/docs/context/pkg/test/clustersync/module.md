# Module atlas

## Responsibility

Test-only builder utility for constructing `hiveinternalv1alpha1.ClusterSync` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hiveinternalv1alpha1.ClusterSync)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hiveinternalv1alpha1.ClusterSync` -- constructs a ClusterSync from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithSyncSetStatus(syncStatus hiveinternalv1alpha1.SyncStatus) Option` -- appends to Status.SyncSets
- `WithSelectorSyncSetStatus(syncStatus hiveinternalv1alpha1.SyncStatus) Option` -- appends to Status.SelectorSyncSets
- `WithCondition(cond hiveinternalv1alpha1.ClusterSyncCondition) Option` -- adds or replaces a status condition
- `WithFirstSuccessTime(firstSuccessTime time.Time) Option` -- sets Status.FirstSuccessTime
- `WithNoFirstSuccessTime() Option` -- clears Status.FirstSuccessTime

## Internal Dependencies

- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hiveinternalv1alpha1.ClusterSync` test fixtures with SyncSet and SelectorSyncSet status entries
- Manages conditions and first-success timestamps
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

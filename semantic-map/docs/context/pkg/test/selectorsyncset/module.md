# Module atlas

## Responsibility

Test builder for `hivev1.SelectorSyncSet` resources. Provides options for name, namespace, generation, label selectors, apply mode/behavior, resources, secrets, and patches.

## Public Interface/API

- `type Option func(*hivev1.SelectorSyncSet)` -- functional option type
- `Build(opts ...Option) *hivev1.SelectorSyncSet` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(name, typer) Builder` -- cluster-scoped (no namespace param)
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` / `WithNamespace(namespace) Option` / `WithGeneration(gen) Option`
- `WithLabelSelector(key, value) Option` -- adds a MatchLabels entry to `Spec.ClusterDeploymentSelector`
- `WithApplyMode(mode) Option` -- sets `Spec.ResourceApplyMode`
- `WithApplyBehavior(behavior) Option` -- sets `Spec.ApplyBehavior`
- `WithResources(objs...) Option` -- sets `Spec.Resources` from MetaRuntimeObjects
- `WithSecrets(secrets...) Option` -- sets `Spec.Secrets`
- `WithPatches(patches...) Option` -- sets `Spec.Patches`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `selectoryncset` (note: Go package name has a typo -- missing "S" in "Sync")
- Standard Hive test builder pattern

## Understanding Score

0.9

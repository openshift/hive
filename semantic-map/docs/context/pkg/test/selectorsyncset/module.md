# Module atlas

## Responsibility

Builder-pattern test fixture for constructing `hivev1.SelectorSyncSet` objects in unit tests. Provides options for configuring label selectors, resources, secrets, patches, apply modes, and apply behaviors.

## Public Interface/API

- `type Option func(*hivev1.SelectorSyncSet)` -- functional option type
- `Build(opts ...Option) *hivev1.SelectorSyncSet` -- constructs a SelectorSyncSet from options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(name string, typer runtime.ObjectTyper) Builder` -- returns a pre-configured builder
- `Generic(opt generic.Option) Option` -- adapts a generic.Option
- `WithName(name string) Option` -- sets object name
- `WithNamespace(namespace string) Option` -- sets object namespace
- `WithGeneration(generation int64) Option` -- sets generation
- `WithLabelSelector(labelKey, labelValue string) Option` -- adds a match label to ClusterDeploymentSelector
- `WithApplyMode(applyMode hivev1.SyncSetResourceApplyMode) Option` -- sets ResourceApplyMode
- `WithApplyBehavior(applyBehavior hivev1.SyncSetApplyBehavior) Option` -- sets ApplyBehavior
- `WithResources(objs ...hivev1.MetaRuntimeObject) Option` -- sets spec resources
- `WithSecrets(secrets ...hivev1.SecretMapping) Option` -- sets spec secrets
- `WithPatches(patches ...hivev1.SyncObjectPatch) Option` -- sets spec patches

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- SelectorSyncSet and related types
- `github.com/openshift/hive/pkg/test/generic` -- shared test builder primitives
- `k8s.io/apimachinery/pkg/runtime` -- ObjectTyper, RawExtension

## Capabilities

- Construct `hivev1.SelectorSyncSet` test fixtures via functional options
- Configure label-based cluster deployment selectors
- Set resources, secrets, patches, apply modes on SelectorSyncSet spec

## Understanding Score

0.85

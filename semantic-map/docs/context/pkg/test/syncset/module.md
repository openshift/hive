# Module atlas

## Responsibility

Builder-pattern test fixture for constructing `hivev1.SyncSet` objects in unit tests. Provides options for resources (typed, YAML, and unstructured), secrets, patches, apply modes, cluster deployment refs, and template enablement.

## Public Interface/API

- `type Option func(*hivev1.SyncSet)` -- functional option type
- `Build(opts ...Option) *hivev1.SyncSet` -- constructs a SyncSet from options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- returns a pre-configured builder
- `Generic(opt generic.Option) Option` -- adapts a generic.Option
- `WithName(name string) Option` -- sets object name
- `WithNamespace(namespace string) Option` -- sets object namespace
- `WithGeneration(generation int64) Option` -- sets generation
- `ForClusterDeployments(clusterDeploymentNames ...string) Option` -- sets ClusterDeploymentRefs
- `WithApplyMode(applyMode hivev1.SyncSetResourceApplyMode) Option` -- sets ResourceApplyMode
- `WithApplyBehavior(applyBehavior hivev1.SyncSetApplyBehavior) Option` -- sets ApplyBehavior
- `WithResources(objs ...hivev1.MetaRuntimeObject) Option` -- sets resources from typed objects
- `WithYAMLResources(objs ...string) Option` -- sets resources from YAML strings (converted to JSON)
- `WithSecrets(secrets ...hivev1.SecretMapping) Option` -- sets spec secrets
- `WithPatches(patches ...hivev1.SyncObjectPatch) Option` -- sets spec patches
- `WithEnablePatchTemplates(on bool) Option` -- enables/disables patch templates
- `WithEnableResourceTemplates(on bool) Option` -- enables/disables resource templates
- `WithUnstructuredResources(objs ...unstructured.Unstructured) Option` -- sets resources from unstructured objects

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- SyncSet and related types
- `github.com/openshift/hive/pkg/test/generic` -- shared test builder primitives
- `k8s.io/api/core/v1` -- LocalObjectReference
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` -- Unstructured type
- `k8s.io/apimachinery/pkg/runtime` -- RawExtension, ObjectTyper
- `sigs.k8s.io/yaml` -- YAML-to-JSON conversion

## Capabilities

- Construct `hivev1.SyncSet` test fixtures via functional options
- Support typed, YAML string, and unstructured resource inputs
- Configure cluster deployment references, apply modes, secrets, patches
- Toggle patch and resource template features

## Understanding Score

0.85

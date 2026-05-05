# Module atlas

## Responsibility

Test builder for `hivev1.SyncSet` resources. Provides options for name, namespace, generation, cluster deployment references, apply mode/behavior, resources (from objects, YAML strings, or unstructured), secrets, patches, and template enablement flags.

## Public Interface/API

- `type Option func(*hivev1.SyncSet)` -- functional option type
- `Build(opts ...Option) *hivev1.SyncSet` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithName(name) Option` / `WithNamespace(namespace) Option` / `WithGeneration(gen) Option`
- `ForClusterDeployments(names...) Option` -- sets `Spec.ClusterDeploymentRefs`
- `WithApplyMode(mode) Option` -- sets `Spec.ResourceApplyMode`
- `WithApplyBehavior(behavior) Option` -- sets `Spec.ApplyBehavior`
- `WithResources(objs...) Option` -- sets resources from MetaRuntimeObjects
- `WithYAMLResources(yamlStrings...) Option` -- sets resources from YAML strings (converted to JSON)
- `WithUnstructuredResources(objs...) Option` -- sets resources from unstructured objects
- `WithSecrets(secrets...) Option` -- sets `Spec.Secrets`
- `WithPatches(patches...) Option` -- sets `Spec.Patches`
- `WithEnablePatchTemplates(on) Option` -- toggles patch templating
- `WithEnableResourceTemplates(on) Option` -- toggles resource templating

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `sigs.k8s.io/yaml` -- YAML-to-JSON conversion

## Capabilities

- **Package**: `syncset`
- Standard Hive test builder pattern; most feature-rich sync-related builder

## Understanding Score

0.9

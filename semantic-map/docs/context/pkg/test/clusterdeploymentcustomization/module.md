# Module atlas

## Responsibility

Test builder for `hivev1.ClusterDeploymentCustomization` resources. Provides options for setting availability status, install-config patches, installer manifest patches, apply-succeeded conditions, and pool/CD references.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeploymentCustomization)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterDeploymentCustomization` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `Available() Option` -- adds Available=True condition
- `Reserved() Option` -- adds Available=False/Reserved condition
- `WithInstallConfigPatch(path, op, value) Option` -- appends an install-config patch entity
- `WithManifestPatch(glob, path, op, value) Option` -- appends an installer manifest patch
- `WithApplySucceeded(reason, changeTime) Option` -- sets or updates ApplySucceeded condition
- `WithPool(name) Option` -- sets `Status.ClusterPoolRef`
- `WithCD(name) Option` -- sets `Status.ClusterDeploymentRef`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`, `k8s.io/apimachinery/pkg/api/meta`, `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `time`

## Capabilities

- **Package**: `clusterdeploymentcustomization`
- Standard Hive test builder pattern

## Understanding Score

0.9

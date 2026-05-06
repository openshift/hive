# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterDeploymentCustomization` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeploymentCustomization)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterDeploymentCustomization` -- constructs a CDC from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `Available() Option` -- adds Available=True condition
- `Reserved() Option` -- adds Available=False/Reserved condition
- `WithInstallConfigPatch(path, op, value string) Option` -- appends an install config patch
- `WithManifestPatch(glob, path, op, value string) Option` -- appends an installer manifest patch with glob selector
- `WithApplySucceeded(reason string, change time.Time) Option` -- sets or updates the ApplySucceeded condition
- `WithPool(name string) Option` -- sets Status.ClusterPoolRef
- `WithCD(name string) Option` -- sets Status.ClusterDeploymentRef

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.ClusterDeploymentCustomization` test fixtures with install config and manifest patches
- Configures availability/reservation status conditions and ApplySucceeded condition
- Sets status references to ClusterPool and ClusterDeployment
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

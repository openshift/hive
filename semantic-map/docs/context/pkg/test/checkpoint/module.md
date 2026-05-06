# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.Checkpoint` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.Checkpoint)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.Checkpoint` -- constructs a Checkpoint from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- returns a builder pre-configured with TypeMeta, ResourceVersion, namespace, and name
- `Generic(opt generic.Option) Option` -- adapts a generic option for Checkpoint
- `WithResourceVersion(resourceVersion string) Option`
- `WithTypeMeta() Option`
- `WithLastBackupChecksum(lastBackupChecksum string) Option`
- `WithLastBackupTime(lastBackupTime metav1.Time) Option`
- `WithLastBackupRef(lastBackupRef hivev1.BackupReference) Option`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.Checkpoint` test fixtures with configurable spec fields (backup checksum, time, ref)
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

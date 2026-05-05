# Module atlas

## Responsibility

Test builder for `hivev1.Checkpoint` resources. Provides a fluent functional-options pattern to construct Checkpoint test fixtures with configurable spec fields (backup checksum, backup time, backup reference).

## Public Interface/API

- `type Option func(*hivev1.Checkpoint)` -- functional option type
- `Build(opts ...Option) *hivev1.Checkpoint` -- constructs a Checkpoint by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions` methods
- `BasicBuilder() Builder` -- returns a bare builder
- `FullBuilder(namespace, name, typer) Builder` -- returns a builder pre-configured with TypeMeta, ResourceVersion, namespace, and name
- `Generic(opt) Option` -- adapts a `generic.Option` to a checkpoint Option
- `WithResourceVersion(rv) Option` -- sets resource version
- `WithTypeMeta() Option` -- sets APIVersion and Kind for Checkpoint
- `WithLastBackupChecksum(checksum) Option` -- sets `Spec.LastBackupChecksum`
- `WithLastBackupTime(time) Option` -- sets `Spec.LastBackupTime`
- `WithLastBackupRef(ref) Option` -- sets `Spec.LastBackupRef`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/test/generic` -- reusable metadata options
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `checkpoint`
- Standard Hive test builder pattern (Option + Build + Builder interface)

## Understanding Score

0.9

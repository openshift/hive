# Module atlas

## Responsibility

Test builder for `hivev1.DNSZone` resources. Provides options for resource versioning, type meta, owner references, label-based ownership, conditions, zone name, and GCP platform configuration.

## Public Interface/API

- `type Option func(*hivev1.DNSZone)` -- functional option type
- `Build(opts ...Option) *hivev1.DNSZone` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithResourceVersion(rv) Option` -- sets resource version
- `WithIncrementedResourceVersion() Option` -- increments resource version by 1
- `WithTypeMeta(typers...) Option` -- populates TypeMeta
- `WithControllerOwnerReference(owner) Option` -- sets controller owner reference
- `WithOwnerReference(owner) Option` -- sets owner reference
- `WithLabelOwner(cd) Option` -- sets ClusterDeploymentName and DNSZoneType labels
- `WithCondition(cond) Option` -- adds or replaces a DNSZoneCondition
- `WithZone(zone) Option` -- sets `Spec.Zone`
- `WithGCPPlatform(zoneName) Option` -- sets GCP spec and status fields

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants` -- label key constants
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- **Package**: `dnszone`
- Standard Hive test builder pattern

## Understanding Score

0.9

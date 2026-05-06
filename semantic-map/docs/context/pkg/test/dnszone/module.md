# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.DNSZone` objects using the functional options pattern.

## Public Interface/API

- `type Option func(*hivev1.DNSZone)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.DNSZone` -- constructs a DNSZone from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithResourceVersion(resourceVersion string) Option`
- `WithIncrementedResourceVersion() Option`
- `WithTypeMeta(typers ...runtime.ObjectTyper) Option`
- `WithControllerOwnerReference(owner metav1.Object) Option`
- `WithOwnerReference(owner hivev1.MetaRuntimeObject) Option`
- `WithLabelOwner(owner *hivev1.ClusterDeployment) Option` -- sets ClusterDeploymentName and DNSZoneType labels
- `WithCondition(cond hivev1.DNSZoneCondition) Option` -- adds or replaces a status condition
- `WithZone(zone string) Option` -- sets Spec.Zone
- `WithGCPPlatform(zoneName string) Option` -- sets GCP spec and status with zone name

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.DNSZone` test fixtures with zone spec, conditions, and owner labels
- Configures GCP platform-specific spec and status fields
- Manages owner references (both controller and non-controller)
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

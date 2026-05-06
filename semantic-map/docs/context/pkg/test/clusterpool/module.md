# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterPool` objects using the functional options pattern. Provides extensive options for pool sizing, platform, claim lifetimes, inventory, and customization.

## Public Interface/API

- `type Option func(*hivev1.ClusterPool)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterPool` -- constructs a ClusterPool from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `ForAWS(credsSecretName, region string) Option` -- sets AWS platform with credentials
- `ForOpenstack(credsSecretName string) Option` -- sets OpenStack platform (clears AWS)
- `WithPullSecret(pullSecretName string) Option`
- `WithSize(size int) Option`
- `WithClusterDeploymentLabels(labels map[string]string) Option`
- `WithPlatform(platform hivev1.Platform) Option`
- `WithBaseDomain(baseDomain string) Option`
- `WithImageSet(clusterImageSetName string) Option`
- `WithInstallConfigSecretTemplateRef(name string) Option`
- `WithInstallAttemptsLimit(ial int32) Option`
- `WithInstallerEnv(ie []corev1.EnvVar) Option`
- `WithMaxSize(size int) Option`
- `WithMaxConcurrent(size int) Option`
- `WithDefaultClaimLifetime(d time.Duration) Option`
- `WithMaximumClaimLifetime(d time.Duration) Option`
- `WithCondition(cond hivev1.ClusterPoolCondition) Option` -- adds or replaces a status condition
- `WithRunningCount(size int) Option`
- `WithInventory(cdcs []string) Option` -- sets inventory entries from CDC names
- `WithCustomizationRef(cdcName string) Option`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/hive/v1/openstack`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`

## Capabilities

- Builds `hivev1.ClusterPool` test fixtures with AWS and OpenStack platform support
- Configures pool sizing (size, max size, max concurrent, running count)
- Manages claim lifetime defaults and maximums
- Sets inventory entries and customization references
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

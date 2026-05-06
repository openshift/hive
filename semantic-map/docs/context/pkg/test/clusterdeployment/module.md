# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterDeployment` objects using the functional options pattern. Provides extensive options covering platform configuration, install state, power state, cluster pool references, and conditions.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeployment)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterDeployment` -- constructs a ClusterDeployment from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, name
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithName(name string) Option`
- `WithNamespace(namespace string) Option`
- `WithLabel(key, value string) Option`
- `WithAnnotation(key, value string) Option`
- `WithPoolVersion(poolVersion string) Option` -- sets cluster pool spec hash annotation
- `WithCondition(cond hivev1.ClusterDeploymentCondition) Option` -- adds or replaces a status condition
- `Broken() Option` -- sets ProvisionStopped=True
- `WithUnclaimedClusterPoolReference(namespace, poolName string) Option`
- `WithClusterPoolReference(namespace, poolName, claimName string) Option`
- `WithClusterProvision(provisionName string) Option`
- `PreserveOnDelete() Option`
- `Installed() Option`
- `Running() Option` -- sets Installed + PowerState Running with backdated CreationTimestamp
- `InstalledTimestamp(instTime time.Time) Option`
- `InstallRestarts(restarts int) Option`
- `WithClusterVersion(version string) Option`
- `WithPowerState(powerState hivev1.ClusterPowerState) Option`
- `WithStatusPowerState(powerState hivev1.ClusterPowerState) Option`
- `WithHibernateAfter(dur time.Duration) Option`
- `WithAWSPlatform(platform *hivev1aws.Platform) Option`
- `WithGCPPlatform(platform *hivev1gcp.Platform) Option`
- `WithAzurePlatform(platform *hivev1azure.Platform) Option`
- `WithIBMCloudPlatform(platform *hivev1ibmcloud.Platform) Option`
- `WithEmptyPlatformStatus() Option`
- `WithAWSPlatformStatus(platformStatus *hivev1aws.PlatformStatus) Option`
- `WithGCPPlatformStatus(platformStatus *hivev1gcp.PlatformStatus) Option`
- `WithClusterMetadata(clusterMetadata *hivev1.ClusterMetadata) Option`
- `WithCustomization(cdcName string) Option`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/apis/hive/v1/azure`
- `github.com/openshift/hive/apis/hive/v1/gcp`
- `github.com/openshift/hive/apis/hive/v1/ibmcloud`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`

## Capabilities

- Builds `hivev1.ClusterDeployment` test fixtures with multi-cloud platform configuration (AWS, GCP, Azure, IBM Cloud)
- Configures install state, power state, hibernation, cluster pool references, and customization references
- Manages status conditions including the "broken" (ProvisionStopped) shortcut
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

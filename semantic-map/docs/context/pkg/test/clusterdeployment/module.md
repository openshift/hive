# Module atlas

## Responsibility

Test builder for `hivev1.ClusterDeployment` resources. The most feature-rich test builder in the suite, offering options for all major ClusterDeployment fields: metadata, conditions, platform configurations (AWS, GCP, Azure, IBMCloud), cluster pool references, installation state, power state, hibernation, provisioning, and cluster metadata.

## Public Interface/API

- `type Option func(*hivev1.ClusterDeployment)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterDeployment` -- constructs a ClusterDeployment by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name, typer) Builder`
- `Generic(opt) Option` -- adapts a `generic.Option`
- **Metadata**: `WithName`, `WithNamespace`, `WithLabel`, `WithAnnotation`, `WithPoolVersion`
- **Conditions**: `WithCondition(cond)`, `Broken()` (sets ProvisionStopped=True)
- **Pool references**: `WithUnclaimedClusterPoolReference`, `WithClusterPoolReference`, `WithCustomization`
- **Provisioning**: `WithClusterProvision(name)`, `InstallRestarts(n)`
- **Install state**: `Installed()`, `Running()`, `InstalledTimestamp(time)`, `WithClusterVersion(v)`
- **Power state**: `WithPowerState`, `WithStatusPowerState`, `WithHibernateAfter(dur)`, `PreserveOnDelete()`
- **Platform spec**: `WithAWSPlatform`, `WithGCPPlatform`, `WithAzurePlatform`, `WithIBMCloudPlatform`
- **Platform status**: `WithEmptyPlatformStatus`, `WithAWSPlatformStatus`, `WithGCPPlatformStatus`
- **Cluster metadata**: `WithClusterMetadata(metadata)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` and platform sub-packages (`aws`, `azure`, `gcp`, `ibmcloud`)
- `github.com/openshift/hive/pkg/constants` -- annotation and label keys
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`, `k8s.io/apimachinery/pkg/apis/meta/v1`, `k8s.io/apimachinery/pkg/runtime`
- `time`

## Capabilities

- **Package**: `clusterdeployment`
- Standard Hive test builder pattern; largest option surface area among the builders

## Understanding Score

0.9

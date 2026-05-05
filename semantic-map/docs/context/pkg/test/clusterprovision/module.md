# Module atlas

## Responsibility

Test builder for `hivev1.ClusterProvision` resources. Provides options for provisioning stage, cluster deployment references, success/failure states, attempt numbering, failure conditions (time, reason, stuck pod), job references, and metadata.

## Public Interface/API

- `type Option func(*hivev1.ClusterProvision)` -- functional option type
- `Build(opts ...Option) *hivev1.ClusterProvision` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, name) Builder` -- note: FullBuilder uses the scheme package directly instead of accepting a typer
- `Generic(opt) Option` -- adapts a `generic.Option`
- `WithClusterDeploymentRef(cdName) Option` -- sets spec ref and label
- `WithStage(stage) Option` -- sets provisioning stage
- `Successful(clusterID, infraID, kubeconfigSecret, passwordSecret) Option` -- marks complete with cluster details
- `Failed() Option` -- sets stage to Failed
- `WithPreviousInfraID(previous) Option` -- sets `Spec.PrevInfraID`
- `Attempt(attempt) Option` -- renames provision and sets attempt number
- `WithFailureTime(time) Option` -- sets failed condition with transition time
- `WithFailureReason(reason) Option` -- sets failed condition with reason
- `WithCreationTimestamp(time) Option` -- sets creation timestamp
- `WithStuckInstallPod() Option` -- sets InstallPodStuck condition
- `WithJob(jobName) Option` -- sets `Status.JobRef`
- `WithMetadata(md) Option` -- sets `Spec.MetadataJSON`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants` -- ClusterDeploymentNameLabel
- `github.com/openshift/hive/pkg/test/generic`
- `github.com/openshift/hive/pkg/util/scheme` -- used by FullBuilder
- `k8s.io/api/core/v1`, `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/utils/ptr`
- `fmt`, `time`

## Capabilities

- **Package**: `clusterprovision`
- Standard Hive test builder pattern; FullBuilder differs from others by using the scheme directly

## Understanding Score

0.9

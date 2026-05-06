# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.ClusterProvision` objects using the functional options pattern. Covers provision stages, success/failure states, and install pod conditions.

## Public Interface/API

- `type Option func(*hivev1.ClusterProvision)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.ClusterProvision` -- constructs a ClusterProvision from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, name string) Builder` -- pre-configured with TypeMeta (from scheme), ResourceVersion, namespace, name, and Initializing stage
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `WithClusterDeploymentRef(cdName string) Option` -- sets ClusterDeploymentRef and label
- `WithStage(stage hivev1.ClusterProvisionStage) Option`
- `Successful(clusterID, infraID, kubeconfigSecretName, passwordSecretName string) Option` -- sets stage to Complete with cluster metadata
- `Failed() Option` -- sets stage to Failed
- `WithPreviousInfraID(previous *string) Option`
- `Attempt(attempt int) Option` -- sets attempt number and renames the provision
- `WithFailureTime(time time.Time) Option` -- marks as failed with a specific transition time
- `WithFailureReason(reason string) Option` -- marks as failed with a specific reason
- `WithCreationTimestamp(time time.Time) Option`
- `WithStuckInstallPod() Option` -- adds InstallPodStuck condition
- `WithJob(jobName string) Option` -- sets Status.JobRef
- `WithMetadata(md string) Option` -- sets Spec.MetadataJSON

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/test/generic`
- `github.com/openshift/hive/pkg/util/scheme`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/utils/ptr`

## Capabilities

- Builds `hivev1.ClusterProvision` test fixtures across all provision stages (Initializing, Complete, Failed)
- Configures successful provision results with cluster ID, infra ID, and secret references
- Models failure scenarios with timestamps, reasons, and stuck install pod conditions
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

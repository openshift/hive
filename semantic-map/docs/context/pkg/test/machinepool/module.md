# Module atlas

## Responsibility

Test-only builder utility for constructing `hivev1.MachinePool` objects using the functional options pattern. Provides extensive options for replicas, autoscaling, labels, taints, and AWS instance configuration.

## Public Interface/API

- `type Option func(*hivev1.MachinePool)` -- functional option type
- `type Builder interface` -- chainable builder with `Build`, `Options`, `GenericOptions` methods
- `Build(opts ...Option) *hivev1.MachinePool` -- constructs a MachinePool from options
- `BasicBuilder() Builder` -- returns an empty builder
- `FullBuilder(namespace, poolName, clusterDeploymentName string, typer runtime.ObjectTyper) Builder` -- pre-configured with TypeMeta, ResourceVersion, namespace, and pool name linked to a ClusterDeployment
- `Generic(opt generic.Option) Option` -- adapts a generic option
- `Deleted() Option` -- sets deletion timestamp
- `WithNamespace(namespace string) Option`
- `WithPoolNameForClusterDeployment(poolName, clusterDeploymentName string) Option` -- sets name as "{cd}-{pool}", links ClusterDeploymentRef
- `WithInitializedStatusConditions() Option` -- replaces conditions with all Unknown-status condition types
- `WithControllerOrdinal(ordinalID int64) Option` -- sets Status.ControlledByReplica
- `WithFinalizer(finalizer string) Option`
- `WithAnnotations(annotations map[string]string) Option`
- `WithReplicas(replicas int64) Option`
- `WithAWSInstanceType(instanceType string) Option`
- `WithLabels(labels map[string]string) Option` -- adds labels to Spec.Labels
- `WithMachineLabels(labels map[string]string) Option` -- adds labels to Spec.MachineLabels
- `WithOwnedLabels(labelKeys ...string) Option` -- appends to Status.OwnedLabels
- `WithOwnedMachineLabels(labelKeys ...string) Option` -- appends to Status.OwnedMachineLabels
- `WithTaints(taints ...corev1.Taint) Option` -- appends to Spec.Taints
- `WithOwnedTaints(taintIDs ...hivev1.TaintIdentifier) Option` -- appends to Status.OwnedTaints
- `WithAutoscaling(min, max int32) Option` -- sets autoscaling config, clears replicas, updates NotEnoughReplicas condition

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws`
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`

## Capabilities

- Builds `hivev1.MachinePool` test fixtures linked to ClusterDeployments
- Configures replicas and autoscaling with min/max
- Manages spec-level and machine-level labels/taints with ownership tracking in status
- Sets initialized status conditions and controller ordinal
- AWS instance type configuration
- Supports generic metadata options via `pkg/test/generic`

## Understanding Score

0.9

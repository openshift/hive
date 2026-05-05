# Module atlas

## Responsibility

Test builder for `hivev1.MachinePool` resources. Provides a comprehensive set of options for pool naming, replicas, autoscaling, AWS instance types, labels (spec-level and machine-level), taints, owned labels/taints tracking, status conditions, controller ordinal, finalizers, annotations, and deletion markers.

## Public Interface/API

- `type Option func(*hivev1.MachinePool)` -- functional option type
- `Build(opts ...Option) *hivev1.MachinePool` -- constructs by applying options
- `type Builder interface` -- fluent builder with `Build`, `Options`, `GenericOptions`
- `BasicBuilder() Builder` / `FullBuilder(namespace, poolName, clusterDeploymentName, typer) Builder` -- FullBuilder sets name as `{cd}-{pool}` and links to the CD
- `Generic(opt) Option` -- adapts a `generic.Option`
- `Deleted() Option` -- sets deletion timestamp
- `WithNamespace(ns) Option` / `WithPoolNameForClusterDeployment(poolName, cdName) Option`
- `WithInitializedStatusConditions() Option` -- sets all standard conditions to Unknown
- `WithControllerOrdinal(ordinalID) Option` -- sets `Status.ControlledByReplica`
- `WithFinalizer(finalizer) Option` / `WithAnnotations(annotations) Option`
- `WithReplicas(n) Option` / `WithAutoscaling(min, max) Option`
- `WithAWSInstanceType(instanceType) Option`
- `WithLabels(labels) Option` / `WithMachineLabels(labels) Option`
- `WithOwnedLabels(keys...) Option` / `WithOwnedMachineLabels(keys...) Option`
- `WithTaints(taints...) Option` / `WithOwnedTaints(taintIDs...) Option`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hive/v1/aws` -- MachinePoolPlatform
- `github.com/openshift/hive/pkg/test/generic`
- `k8s.io/api/core/v1`, `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/utils/ptr`
- `fmt`

## Capabilities

- **Package**: `machinepool`
- Standard Hive test builder pattern; one of the larger builders with status-aware options

## Understanding Score

0.9

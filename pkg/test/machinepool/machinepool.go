package machinepool

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.MachinePool)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.MachinePool {
	retval := &hivev1.MachinePool{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.MachinePool

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace, poolName, clusterDeploymentName string, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
	).Options(
		WithPoolNameForClusterDeployment(poolName, clusterDeploymentName),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *hivev1.MachinePool {
	return Build(append(b.options, opts...)...)
}

func (b *builder) Options(opts ...Option) Builder {
	return &builder{
		options: append(b.options, opts...),
	}
}

func (b *builder) GenericOptions(opts ...generic.Option) Builder {
	options := make([]Option, len(opts))
	for i, o := range opts {
		options[i] = Generic(o)
	}
	return b.Options(options...)
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(machinePool *hivev1.MachinePool) {
		opt(machinePool)
	}
}

func Deleted() Option {
	return Generic(generic.Deleted())
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return Generic(generic.WithNamespace(namespace))
}

// WithName sets the object.Name field when building an object with Build.
func WithPoolNameForClusterDeployment(poolName, clusterDeploymentName string) Option {
	return func(machinePool *hivev1.MachinePool) {
		machinePool.Name = fmt.Sprintf("%s-%s", clusterDeploymentName, poolName)
		machinePool.Spec.ClusterDeploymentRef = corev1.LocalObjectReference{Name: clusterDeploymentName}
		machinePool.Spec.Name = poolName

	}
}

// WithInitializedStatusConditions returns an Option that *replaces* status conditions
// with the set of initialized ("Unknown") conditions.
func WithInitializedStatusConditions() Option {
	return func(mp *hivev1.MachinePool) {
		mp.Status.Conditions = []hivev1.MachinePoolCondition{
			{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.NotEnoughReplicasMachinePoolCondition,
			},
			{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.NoMachinePoolNameLeasesAvailable,
			},
			{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
			},
			{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
			},
		}
	}
}

func WithFinalizer(finalizer string) Option {
	return Generic(generic.WithFinalizer(finalizer))
}

func WithAnnotations(annotations map[string]string) Option {
	return func(mp *hivev1.MachinePool) {
		for k, v := range annotations {
			generic.WithAnnotation(k, v)(mp)
		}
	}
}

func WithReplicas(replicas int64) Option {
	return func(mp *hivev1.MachinePool) {
		mp.Spec.Replicas = pointer.Int64(replicas)
	}
}

func WithAWSInstanceType(instanceType string) Option {
	return func(mp *hivev1.MachinePool) {
		if mp.Spec.Platform.AWS == nil {
			mp.Spec.Platform.AWS = &aws.MachinePoolPlatform{}
		}
		mp.Spec.Platform.AWS.InstanceType = instanceType
	}
}

// WithLabels returns an Option that *adds* labels on the target MachinePool's
// *Spec* (not its metadata!)
// Existing labels are not removed. When keys conflict, those given in the most
// recent WithLabels will win.
func WithLabels(labels map[string]string) Option {
	return func(mp *hivev1.MachinePool) {
		if mp.Spec.Labels == nil {
			mp.Spec.Labels = labels
		} else {
			for k, v := range labels {
				mp.Spec.Labels[k] = v
			}
		}
	}
}

// WithOwnedLabels returns an Option that *appends* labelKeys to the target's
// Status.OwnedLabels. Existing keys are not removed. You are responsible for
// avoiding duplicates.
func WithOwnedLabels(labelKeys ...string) Option {
	return func(mp *hivev1.MachinePool) {
		if mp.Status.OwnedLabels == nil {
			mp.Status.OwnedLabels = labelKeys
		} else {
			mp.Status.OwnedLabels = append(mp.Status.OwnedLabels, labelKeys...)
		}
	}
}

// WithTaints returns an Option that *appends* taints on the target MachinePool's
// *Spec* (not its metadata!)
// Existing taints are not removed. You are responsible for avoiding duplicates.
func WithTaints(taints ...corev1.Taint) Option {
	return func(mp *hivev1.MachinePool) {
		if mp.Spec.Taints == nil {
			mp.Spec.Taints = taints
		} else {
			mp.Spec.Taints = append(mp.Spec.Taints, taints...)
		}
	}
}

// WithOwnedTaints returns an Option that *appends* taint identifiers on the
// target MachinePool's Status.OwnedTaints. Existing taint identifiers are not
// removed. You are responsible for avoiding duplicates.
func WithOwnedTaints(taintIDs ...hivev1.TaintIdentifier) Option {
	return func(mp *hivev1.MachinePool) {
		if mp.Status.OwnedTaints == nil {
			mp.Status.OwnedTaints = taintIDs
		} else {
			mp.Status.OwnedTaints = append(mp.Status.OwnedTaints, taintIDs...)
		}
	}
}

func WithAutoscaling(min, max int32) Option {
	return func(mp *hivev1.MachinePool) {
		mp.Spec.Replicas = nil
		mp.Spec.Autoscaling = &hivev1.MachinePoolAutoscaling{
			MinReplicas: min,
			MaxReplicas: max,
		}
		for i, cond := range mp.Status.Conditions {
			// Condition will always be present because it is initialized in testMachinePool
			if cond.Type == hivev1.NotEnoughReplicasMachinePoolCondition {
				cond.Status = corev1.ConditionFalse
				cond.Reason = "EnoughReplicas"
				mp.Status.Conditions[i] = cond
			}
		}
	}
}

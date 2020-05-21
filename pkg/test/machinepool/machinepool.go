package machinepool

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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

// BuildFull builds a MachinePool with the specified pool name for the ClusterDeployment, namespace, and supplied options.
// This will also fill out the type meta and resource version for the MachinePool.
func BuildFull(namespace, poolName, cdName string, options ...Option) *hivev1.MachinePool {
	options = append(
		[]Option{
			Generic(generic.WithTypeMeta()),
			Generic(generic.WithResourceVersion("1")),
			Generic(generic.WithNamespace(namespace)),
			WithPoolNameForClusterDeployment(poolName, cdName),
		},
		options...,
	)
	return Build(options...)
}

// Generic allows common functions applicable to all objects to be used as Options to Build
func Generic(opt generic.Option) Option {
	return func(machinePool *hivev1.MachinePool) {
		opt(machinePool)
	}
}

// WithName sets the object.Name field when building an object with Build.
func WithPoolNameForClusterDeployment(poolName, clusterDeploymentName string) Option {
	return func(machinePool *hivev1.MachinePool) {
		machinePool.Name = fmt.Sprintf("%s-%s", clusterDeploymentName, poolName)
		machinePool.Spec.ClusterDeploymentRef = corev1.LocalObjectReference{Name: clusterDeploymentName}
		machinePool.Spec.Name = poolName

	}
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return Generic(generic.WithNamespace(namespace))
}

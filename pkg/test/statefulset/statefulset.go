package statefulset

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*appsv1.StatefulSet)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *appsv1.StatefulSet {
	retval := &appsv1.StatefulSet{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *appsv1.StatefulSet

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace string, name hivev1.DeploymentName, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
		generic.WithName(string(name)),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *appsv1.StatefulSet {
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
	return func(statefulset *appsv1.StatefulSet) {
		opt(statefulset)
	}
}

// WithName sets the object.Name field when building an object with Build.
func WithName(name string) Option {
	return Generic(generic.WithName(name))
}

// WithNamespace sets the object.Namespace field when building an object with Build.
func WithNamespace(namespace string) Option {
	return Generic(generic.WithNamespace(namespace))
}

// WithReplicas sets the spec.Replicas field when building an object with Build.
func WithReplicas(replicas int32) Option {
	return func(statefulset *appsv1.StatefulSet) {
		statefulset.Spec.Replicas = ptr.To(replicas)
	}
}

// WithCurrentReplicas sets the status.CurrentReplicas field when building an object with Build.
func WithCurrentReplicas(currentReplicas int32) Option {
	return func(statefulset *appsv1.StatefulSet) {
		statefulset.Status.CurrentReplicas = currentReplicas
	}
}

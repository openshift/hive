package job

import (
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*batchv1.Job)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *batchv1.Job {
	retval := &batchv1.Job{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *batchv1.Job

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace, name string, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
		generic.WithName(name),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *batchv1.Job {
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
	return func(checkpoint *batchv1.Job) {
		opt(checkpoint)
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

package namespace

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*corev1.Namespace)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *corev1.Namespace {
	retval := &corev1.Namespace{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *corev1.Namespace

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(name string, typer runtime.ObjectTyper) Builder {
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(typer),
		generic.WithResourceVersion("1"),
		generic.WithName(name),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *corev1.Namespace {
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
	return func(checkpoint *corev1.Namespace) {
		opt(checkpoint)
	}
}

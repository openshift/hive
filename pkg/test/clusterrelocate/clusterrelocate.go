package clusterrelocate

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterRelocate)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterRelocate {
	retval := &hivev1.ClusterRelocate{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterRelocate

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

func (b *builder) Build(opts ...Option) *hivev1.ClusterRelocate {
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
	return func(clusterRelocate *hivev1.ClusterRelocate) {
		opt(clusterRelocate)
	}
}

func WithKubeconfigSecret(namespace, name string) Option {
	return func(clusterRelocate *hivev1.ClusterRelocate) {
		clusterRelocate.Spec.KubeconfigSecretRef = hivev1.KubeconfigSecretReference{
			Namespace: namespace,
			Name:      name,
		}
	}
}

func WithClusterDeploymentSelector(key, value string) Option {
	return func(clusterRelocate *hivev1.ClusterRelocate) {
		clusterRelocate.Spec.ClusterDeploymentSelector = metav1.LabelSelector{
			MatchLabels: map[string]string{key: value},
		}
	}
}

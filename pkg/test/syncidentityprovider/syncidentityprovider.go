package syncidentityprovider

import (
	openshiftapiv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.SyncIdentityProvider)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.SyncIdentityProvider {
	retval := &hivev1.SyncIdentityProvider{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.SyncIdentityProvider

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

func (b *builder) Build(opts ...Option) *hivev1.SyncIdentityProvider {
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
	return func(syncIdentityProvider *hivev1.SyncIdentityProvider) {
		opt(syncIdentityProvider)
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

func ForClusterDeployments(clusterDeploymentNames ...string) Option {
	return func(syncIdentityProvider *hivev1.SyncIdentityProvider) {
		syncIdentityProvider.Spec.ClusterDeploymentRefs = make([]corev1.LocalObjectReference, len(clusterDeploymentNames))
		for i, name := range clusterDeploymentNames {
			syncIdentityProvider.Spec.ClusterDeploymentRefs[i] = corev1.LocalObjectReference{Name: name}
		}
	}
}

func ForIdentities(names ...string) Option {
	return func(syncIdentityProvider *hivev1.SyncIdentityProvider) {
		syncIdentityProvider.Spec.IdentityProviders = make([]openshiftapiv1.IdentityProvider, len(names))
		for i, name := range names {
			syncIdentityProvider.Spec.IdentityProviders[i] = openshiftapiv1.IdentityProvider{Name: name}
		}
	}
}

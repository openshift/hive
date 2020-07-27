package clusterdeployment

import (
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterDeployment)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterDeployment {
	retval := &hivev1.ClusterDeployment{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterDeployment

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

func (b *builder) Build(opts ...Option) *hivev1.ClusterDeployment {
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
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		opt(clusterDeployment)
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

// WithLabel sets the specified label on the supplied object.
func WithLabel(key, value string) Option {
	return Generic(generic.WithLabel(key, value))
}

// WithCondition adds the specified condition to the ClusterDeployment
func WithCondition(cond hivev1.ClusterDeploymentCondition) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		for i, c := range clusterDeployment.Status.Conditions {
			if c.Type == cond.Type {
				clusterDeployment.Status.Conditions[i] = cond
				return
			}
		}
		clusterDeployment.Status.Conditions = append(clusterDeployment.Status.Conditions, cond)
	}
}

func WithUnclaimedClusterPoolReference(namespace, poolName string) Option {
	return WithClusterPoolReference(namespace, poolName, "")
}

func WithClusterPoolReference(namespace, poolName, claimName string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ClusterPoolRef = &hivev1.ClusterPoolReference{
			Namespace: namespace,
			PoolName:  poolName,
			ClaimName: claimName,
		}
	}
}

func Installed() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
	}
}

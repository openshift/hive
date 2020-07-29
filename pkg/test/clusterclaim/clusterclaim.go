package clusterclaim

import (
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterClaim)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterClaim {
	retval := &hivev1.ClusterClaim{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterClaim

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

func (b *builder) Build(opts ...Option) *hivev1.ClusterClaim {
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
	return func(clusterClaim *hivev1.ClusterClaim) {
		opt(clusterClaim)
	}
}

func WithPool(poolName string) Option {
	return func(clusterClaim *hivev1.ClusterClaim) {
		clusterClaim.Spec.ClusterPoolName = poolName
	}
}

func WithCluster(clusterName string) Option {
	return func(clusterClaim *hivev1.ClusterClaim) {
		clusterClaim.Spec.Namespace = clusterName
	}
}

func WithSubjects(subjects []rbacv1.Subject) Option {
	return func(clusterClaim *hivev1.ClusterClaim) {
		clusterClaim.Spec.Subjects = subjects
	}
}

// WithCondition adds the specified condition to the ClusterClaim
func WithCondition(cond hivev1.ClusterClaimCondition) Option {
	return func(clusterClaim *hivev1.ClusterClaim) {
		for i, c := range clusterClaim.Status.Conditions {
			if c.Type == cond.Type {
				clusterClaim.Status.Conditions[i] = cond
				return
			}
		}
		clusterClaim.Status.Conditions = append(clusterClaim.Status.Conditions, cond)
	}
}

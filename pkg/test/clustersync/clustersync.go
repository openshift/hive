package clusterSync

import (
	"k8s.io/apimachinery/pkg/runtime"

	hiveinternalv1alpha1 "github.com/openshift/hive/pkg/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hiveinternalv1alpha1.ClusterSync)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hiveinternalv1alpha1.ClusterSync {
	retval := &hiveinternalv1alpha1.ClusterSync{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hiveinternalv1alpha1.ClusterSync

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

func (b *builder) Build(opts ...Option) *hiveinternalv1alpha1.ClusterSync {
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
	return func(clusterSync *hiveinternalv1alpha1.ClusterSync) {
		opt(clusterSync)
	}
}

func WithSyncSetStatus(syncStatus hiveinternalv1alpha1.SyncStatus) Option {
	return func(clusterSync *hiveinternalv1alpha1.ClusterSync) {
		clusterSync.Status.SyncSets = append(clusterSync.Status.SyncSets, syncStatus)
	}
}

func WithSelectorSyncSetStatus(syncStatus hiveinternalv1alpha1.SyncStatus) Option {
	return func(clusterSync *hiveinternalv1alpha1.ClusterSync) {
		clusterSync.Status.SelectorSyncSets = append(clusterSync.Status.SyncSets, syncStatus)
	}
}

// WithCondition adds the specified condition to the ClusterSync
func WithCondition(cond hiveinternalv1alpha1.ClusterSyncCondition) Option {
	return func(clusterSync *hiveinternalv1alpha1.ClusterSync) {
		for i, c := range clusterSync.Status.Conditions {
			if c.Type == cond.Type {
				clusterSync.Status.Conditions[i] = cond
				return
			}
		}
		clusterSync.Status.Conditions = append(clusterSync.Status.Conditions, cond)
	}
}

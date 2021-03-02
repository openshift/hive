package selectoryncset

import (
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.SelectorSyncSet)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.SelectorSyncSet {
	retval := &hivev1.SelectorSyncSet{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.SelectorSyncSet

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

func (b *builder) Build(opts ...Option) *hivev1.SelectorSyncSet {
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
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		opt(selectorSyncSet)
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

func WithGeneration(generation int64) Option {
	return Generic(generic.WithGeneration(generation))
}

func WithLabelSelector(labelKey, labelValue string) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		if selectorSyncSet.Spec.ClusterDeploymentSelector.MatchLabels == nil {
			selectorSyncSet.Spec.ClusterDeploymentSelector.MatchLabels = make(map[string]string, 1)
		}
		selectorSyncSet.Spec.ClusterDeploymentSelector.MatchLabels[labelKey] = labelValue
	}
}

func WithApplyMode(applyMode hivev1.SyncSetResourceApplyMode) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		selectorSyncSet.Spec.ResourceApplyMode = applyMode
	}
}

func WithApplyBehavior(applyBehavior hivev1.SyncSetApplyBehavior) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		selectorSyncSet.Spec.ApplyBehavior = applyBehavior
	}
}

func WithResources(objs ...hivev1.MetaRuntimeObject) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		selectorSyncSet.Spec.Resources = make([]runtime.RawExtension, len(objs))
		for i, obj := range objs {
			selectorSyncSet.Spec.Resources[i].Object = obj
		}
	}
}

func WithSecrets(secrets ...hivev1.SecretMapping) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		selectorSyncSet.Spec.Secrets = secrets
	}
}

func WithPatches(patches ...hivev1.SyncObjectPatch) Option {
	return func(selectorSyncSet *hivev1.SelectorSyncSet) {
		selectorSyncSet.Spec.Patches = patches
	}
}

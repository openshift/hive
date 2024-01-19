package syncset

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.SyncSet)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.SyncSet {
	retval := &hivev1.SyncSet{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.SyncSet

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

func (b *builder) Build(opts ...Option) *hivev1.SyncSet {
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
	return func(syncSet *hivev1.SyncSet) {
		opt(syncSet)
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

func ForClusterDeployments(clusterDeploymentNames ...string) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.ClusterDeploymentRefs = make([]corev1.LocalObjectReference, len(clusterDeploymentNames))
		for i, name := range clusterDeploymentNames {
			syncSet.Spec.ClusterDeploymentRefs[i] = corev1.LocalObjectReference{Name: name}
		}
	}
}

func WithApplyMode(applyMode hivev1.SyncSetResourceApplyMode) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.ResourceApplyMode = applyMode
	}
}

func WithApplyBehavior(applyBehavior hivev1.SyncSetApplyBehavior) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.ApplyBehavior = applyBehavior
	}
}

func WithResources(objs ...hivev1.MetaRuntimeObject) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.Resources = make([]runtime.RawExtension, len(objs))
		for i, obj := range objs {
			syncSet.Spec.Resources[i].Object = obj
		}
	}
}

// Each resource is a separate string. Don't pass in one string with a yaml list.
func WithYAMLResources(objs ...string) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.Resources = make([]runtime.RawExtension, len(objs))
		for i, obj := range objs {
			jsonObj, err := yaml.YAMLToJSON([]byte(obj))
			if err != nil {
				panic(err)
			}
			syncSet.Spec.Resources[i].Raw = jsonObj
		}
	}
}

func WithSecrets(secrets ...hivev1.SecretMapping) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.Secrets = secrets
	}
}

func WithPatches(patches ...hivev1.SyncObjectPatch) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.Patches = patches
	}
}

func WithResourceParametersEnabled(on bool) Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Spec.EnableResourceTemplates = on
	}
}

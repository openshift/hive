package clusterdeploymentcustomization

import (
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	conditionsv1 "github.com/openshift/custom-resource-status/conditions/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/hive/pkg/test/generic"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterDeploymentCustomization)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterDeploymentCustomization {
	retval := &hivev1.ClusterDeploymentCustomization{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterDeploymentCustomization

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

func (b *builder) Build(opts ...Option) *hivev1.ClusterDeploymentCustomization {
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
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		opt(cdc)
	}
}

func Available() Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.Conditions = append(cdc.Status.Conditions, conditionsv1.Condition{
			Type:    conditionsv1.ConditionAvailable,
			Status:  corev1.ConditionTrue,
			Reason:  "Available",
			Message: "available",
		})
	}
}

func Reserved() Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.Conditions = append(cdc.Status.Conditions, conditionsv1.Condition{
			Type:    conditionsv1.ConditionAvailable,
			Status:  corev1.ConditionFalse,
			Reason:  "Reserved",
			Message: "reserved",
		})
	}
}

func WithPatch(path, op, value string) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Spec.InstallConfigPatches = append(cdc.Spec.InstallConfigPatches, hivev1.PatchEntity{
			Path:  path,
			Op:    op,
			Value: value,
		})
	}
}

func WithLastStatus(status hivev1.LastApplyStatusType) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.LastApplyStatus = status
	}
}

func WithLastTime(lastTime time.Time) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.LastApplyTime = metav1.NewTime(lastTime)
	}
}

func WithPool(name string) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.ClusterPoolRef = &corev1.LocalObjectReference{Name: name}
	}
}

func WithCD(name string) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.ClusterDeploymentRef = &corev1.LocalObjectReference{Name: name}
	}
}

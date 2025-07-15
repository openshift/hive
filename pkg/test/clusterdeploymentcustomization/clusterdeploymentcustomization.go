package clusterdeploymentcustomization

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"

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
		cdc.Status.Conditions = append(cdc.Status.Conditions, metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionTrue,
			Reason:  "Available",
			Message: "available",
		})
	}
}

func Reserved() Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Status.Conditions = append(cdc.Status.Conditions, metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionFalse,
			Reason:  "Reserved",
			Message: "reserved",
		})
	}
}

func WithInstallConfigPatch(path, op, value string) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Spec.InstallConfigPatches = append(cdc.Spec.InstallConfigPatches, hivev1.PatchEntity{
			Path:  path,
			Op:    op,
			Value: value,
		})
	}
}

func WithManifestPatch(glob, path, op, value string) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		cdc.Spec.InstallerManifestPatches = append(cdc.Spec.InstallerManifestPatches, hivev1.InstallerManifestPatch{
			ManifestSelector: hivev1.ManifestSelector{
				Glob: glob,
			},
			Patches: []hivev1.PatchEntity{
				{
					Path:  path,
					Op:    op,
					Value: value,
				},
			},
		})
	}
}

func WithApplySucceeded(reason string, change time.Time) Option {
	return func(cdc *hivev1.ClusterDeploymentCustomization) {
		status := metav1.ConditionTrue
		if reason != hivev1.CustomizationApplyReasonSucceeded {
			status = metav1.ConditionFalse
		}

		if cdc.Status.Conditions == nil {
			cdc.Status.Conditions = []metav1.Condition{}
		}
		existingCondition := meta.FindStatusCondition(cdc.Status.Conditions, hivev1.ApplySucceededCondition)
		if existingCondition == nil {
			newCondition := metav1.Condition{
				Type:    hivev1.ApplySucceededCondition,
				Status:  status,
				Reason:  reason,
				Message: reason,
			}
			newCondition.LastTransitionTime = metav1.NewTime(change)
			cdc.Status.Conditions = append(cdc.Status.Conditions, newCondition)
		} else {
			existingCondition.LastTransitionTime = metav1.NewTime(change)
			existingCondition.Status = status
			existingCondition.Reason = reason
		}
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

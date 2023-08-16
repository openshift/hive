package clusterprovision

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

// Option defines a function signature for any function that wants to be passed into Build
type Option func(*hivev1.ClusterProvision)

// Build runs each of the functions passed in to generate the object.
func Build(opts ...Option) *hivev1.ClusterProvision {
	retval := &hivev1.ClusterProvision{}
	for _, o := range opts {
		o(retval)
	}

	return retval
}

type Builder interface {
	Build(opts ...Option) *hivev1.ClusterProvision

	Options(opts ...Option) Builder

	GenericOptions(opts ...generic.Option) Builder
}

func BasicBuilder() Builder {
	return &builder{}
}

func FullBuilder(namespace, name string) Builder {
	scheme := scheme.GetScheme()
	b := &builder{}
	return b.GenericOptions(
		generic.WithTypeMeta(scheme),
		generic.WithResourceVersion("1"),
		generic.WithNamespace(namespace),
		generic.WithName(name),
	).Options(
		WithStage(hivev1.ClusterProvisionStageInitializing),
	)
}

type builder struct {
	options []Option
}

func (b *builder) Build(opts ...Option) *hivev1.ClusterProvision {
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
	return func(clusterProvision *hivev1.ClusterProvision) {
		opt(clusterProvision)
	}
}

func WithClusterDeploymentRef(cdName string) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		Generic(generic.WithLabel(constants.ClusterDeploymentNameLabel, cdName))(clusterProvision)
		clusterProvision.Spec.ClusterDeploymentRef = corev1.LocalObjectReference{
			Name: cdName,
		}
	}
}

func WithStage(stage hivev1.ClusterProvisionStage) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		clusterProvision.Spec.Stage = stage
	}
}

func Successful(clusterID, infraID, kubeconfigSecretName, passwordSecretName string) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		clusterProvision.Spec.Stage = hivev1.ClusterProvisionStageComplete
		clusterProvision.Spec.ClusterID = pointer.String(clusterID)
		clusterProvision.Spec.InfraID = pointer.String(infraID)
		clusterProvision.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: kubeconfigSecretName}
		clusterProvision.Spec.AdminPasswordSecretRef = &corev1.LocalObjectReference{Name: passwordSecretName}
	}
}

func Failed() Option {
	return WithStage(hivev1.ClusterProvisionStageFailed)
}

func Attempt(attempt int) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		// Rename the ClusterProvision according to the attempt number
		Generic(generic.WithName(fmt.Sprintf("%s-%02d", clusterProvision.Name, attempt)))(clusterProvision)
		clusterProvision.Spec.Attempt = attempt
	}
}

func WithFailureTime(time time.Time) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		Failed()(clusterProvision)
		clusterProvision.Status.Conditions = []hivev1.ClusterProvisionCondition{
			{
				Type:               hivev1.ClusterProvisionFailedCondition,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(time),
			},
		}

	}
}

func WithFailureReason(reason string) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		Failed()(clusterProvision)
		clusterProvision.Status.Conditions = []hivev1.ClusterProvisionCondition{
			{
				Type:   hivev1.ClusterProvisionFailedCondition,
				Status: corev1.ConditionTrue,
				Reason: reason,
			},
		}

	}
}

func WithCreationTimestamp(time time.Time) Option {
	return Generic(generic.WithCreationTimestamp(time))
}

func WithStuckInstallPod() Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		clusterProvision.Status.Conditions = []hivev1.ClusterProvisionCondition{
			{
				Type:    hivev1.InstallPodStuckCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "PodInPendingPhase",
				Message: "pod is in pending phase",
			},
		}
	}
}

func WithJob(jobName string) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		clusterProvision.Status.JobRef = &corev1.LocalObjectReference{
			Name: jobName,
		}
	}
}

func WithMetadata(md string) Option {
	return func(clusterProvision *hivev1.ClusterProvision) {
		clusterProvision.Spec.MetadataJSON = ([]byte)(md)
	}
}

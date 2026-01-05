package clusterdeployment

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	"github.com/openshift/hive/pkg/constants"
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

// WithAnnotation sets the specified annotation on the supplied object.
func WithAnnotation(key, value string) Option {
	return Generic(generic.WithAnnotation(key, value))
}

// WithPoolVersion sets the cluster pool spec hash annotation on the supplied object.
func WithPoolVersion(poolVersion string) Option {
	return WithAnnotation(constants.ClusterDeploymentPoolSpecHashAnnotation, poolVersion)
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

// Broken uses ProvisionStopped=True to make the CD be recognized as broken.
func Broken() Option {
	return WithCondition(hivev1.ClusterDeploymentCondition{
		Type:   hivev1.ProvisionStoppedCondition,
		Status: corev1.ConditionTrue,
	})
}

func WithUnclaimedClusterPoolReference(namespace, poolName string) Option {
	return WithClusterPoolReference(namespace, poolName, "")
}

func WithClusterPoolReference(namespace, poolName, claimName string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ClusterPoolRef = &hivev1.ClusterPoolReference{
			Namespace: namespace,
			PoolName:  poolName,
		}
		if claimName != "" {
			clusterDeployment.Spec.ClusterPoolRef.ClaimName = claimName
			now := metav1.Now()
			clusterDeployment.Spec.ClusterPoolRef.ClaimedTimestamp = &now
		}
	}
}

func WithClusterProvision(provisionName string) Option {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: provisionName}
	}
}

func PreserveOnDelete() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.PreserveOnDelete = true
	}
}

func Installed() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
	}
}

func Running() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
		clusterDeployment.Spec.PowerState = hivev1.ClusterPowerStateRunning
		clusterDeployment.Status.PowerState = hivev1.ClusterPowerStateRunning
		// A little hack to make sure tests don't unexpectedly start swapping which clusters are
		// running: since we start the oldest clusters first, fake the creation time of this
		// cluster to be "old".
		clusterDeployment.CreationTimestamp = metav1.NewTime(time.Now().Add(-6 * time.Hour))
	}
}

func InstalledTimestamp(instTime time.Time) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Installed = true
		clusterDeployment.Status.InstalledTimestamp = &metav1.Time{Time: instTime}
	}
}

func InstallRestarts(restarts int) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Status.InstallRestarts = restarts
	}
}

func WithClusterVersion(version string) Option {
	return Generic(generic.WithLabel(constants.VersionLabel, version))
}

func WithPowerState(powerState hivev1.ClusterPowerState) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.PowerState = powerState
	}
}

func WithStatusPowerState(powerState hivev1.ClusterPowerState) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Status.PowerState = powerState
	}
}

func WithHibernateAfter(dur time.Duration) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.HibernateAfter = &metav1.Duration{Duration: dur}
	}
}

// WithAWSPlatform sets the specified aws platform on the supplied object.
func WithAWSPlatform(platform *hivev1aws.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.AWS = platform
	}
}

// WithGCPPlatform sets the specified gcp platform on the supplied object.
func WithGCPPlatform(platform *hivev1gcp.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.GCP = platform
	}
}

// WithAzurePlatform sets the specified azure platform on the supplied object.
func WithAzurePlatform(platform *hivev1azure.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.Azure = platform
	}
}

// WithIBMCloudPlatform sets the specified IBM Cloud platform on the cd.
func WithIBMCloudPlatform(platform *hivev1ibmcloud.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.IBMCloud = platform
	}
}

// WithAWSPlatformStatus sets the specified aws platform status on the supplied object.
func WithEmptyPlatformStatus() Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Status.Platform = &hivev1.PlatformStatus{}
	}
}

// WithAWSPlatformStatus sets the specified aws platform on the supplied object.
func WithAWSPlatformStatus(platformStatus *hivev1aws.PlatformStatus) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		if clusterDeployment.Status.Platform == nil {
			clusterDeployment.Status.Platform = &hivev1.PlatformStatus{}
		}
		clusterDeployment.Status.Platform.AWS = platformStatus
	}
}

// WithOpenStackPlatform sets the specified OpenStack platform on the cd.
func WithOpenStackPlatform(platform *hivev1openstack.Platform) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.Platform.OpenStack = platform
	}
}

// WithGCPPlatformStatus sets the specified aws platform on the supplied object.
func WithGCPPlatformStatus(platformStatus *hivev1gcp.PlatformStatus) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		if clusterDeployment.Status.Platform == nil {
			clusterDeployment.Status.Platform = &hivev1.PlatformStatus{}
		}
		clusterDeployment.Status.Platform.GCP = platformStatus
	}
}

// WithClusterMetadata sets the specified cluster metadata on the cd.
func WithClusterMetadata(clusterMetadata *hivev1.ClusterMetadata) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ClusterMetadata = clusterMetadata
	}
}

func WithCustomization(cdcName string) Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Spec.ClusterPoolRef.CustomizationRef = &corev1.LocalObjectReference{Name: cdcName}
	}
}

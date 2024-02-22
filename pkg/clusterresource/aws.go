package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"

	installertypes "github.com/openshift/installer/pkg/types"
	awsinstallertypes "github.com/openshift/installer/pkg/types/aws"
)

const (
	AWSInstanceTypeDefault = "m6a.xlarge"
	volumeSize             = 120
	volumeType             = "gp3"
)

var _ CloudBuilder = (*AWSCloudBuilder)(nil)

// AWSCloudBuilder encapsulates cluster artifact generation logic specific to AWS.
type AWSCloudBuilder struct {
	// AccessKeyID is the AWS access key ID.
	AccessKeyID string
	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string

	RoleARN, ExternalID string

	// UserTags are user-provided tags to add to resources.
	UserTags map[string]string
	// Region is the AWS region to which to install the cluster
	Region string

	// Instance type for masters and workers, unless the latter is overridden by WorkerInstanceType
	InstanceType string

	// Instance type for workers only. If empty, InstanceType is used.
	WorkerInstanceType string

	PrivateLink bool
}

func NewAWSCloudBuilderFromSecret(credsSecret *corev1.Secret) *AWSCloudBuilder {
	accessKeyID := credsSecret.Data[constants.AWSAccessKeyIDSecretKey]
	secretAccessKey := credsSecret.Data[constants.AWSSecretAccessKeySecretKey]
	return &AWSCloudBuilder{
		AccessKeyID:     string(accessKeyID),
		SecretAccessKey: string(secretAccessKey),
	}
}

func NewAWSCloudBuilderFromAssumeRole(role *hivev1aws.AssumeRole) *AWSCloudBuilder {
	return &AWSCloudBuilder{
		RoleARN:    role.RoleARN,
		ExternalID: role.ExternalID,
	}
}

func (p *AWSCloudBuilder) GenerateCredentialsSecret(o *Builder) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.CredsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			constants.AWSAccessKeyIDSecretKey:     p.AccessKeyID,
			constants.AWSSecretAccessKeySecretKey: p.SecretAccessKey,
		},
	}
}

func (p *AWSCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	plat := hivev1.Platform{
		AWS: &hivev1aws.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region:   p.Region,
			UserTags: p.UserTags,
			PrivateLink: &hivev1aws.PrivateLinkAccess{
				Enabled: p.PrivateLink,
			},
		},
	}
	return plat
}

func (p *AWSCloudBuilder) GenerateCloudObjects(o *Builder) []runtime.Object {
	return []runtime.Object{}
}

func (p *AWSCloudBuilder) workerInstanceType() string {
	if p.WorkerInstanceType != "" {
		return p.WorkerInstanceType
	}
	return p.InstanceType
}

func (p *AWSCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.AWS = &hivev1aws.MachinePoolPlatform{
		InstanceType: p.workerInstanceType(),
		EC2RootVolume: hivev1aws.EC2RootVolume{
			Size: volumeSize,
			Type: volumeType,
		},
	}

}

func (p *AWSCloudBuilder) addInstallConfigPlatform(o *Builder, ic *installertypes.InstallConfig) {
	// Inject platform details into InstallConfig:
	ic.Platform = installertypes.Platform{
		AWS: &awsinstallertypes.Platform{
			Region: p.Region,
		},
	}

	ec2 := awsinstallertypes.EC2RootVolume{
		Size: volumeSize,
		Type: volumeType,
	}
	ic.ControlPlane.Platform.AWS = &awsinstallertypes.MachinePool{
		InstanceType:  p.InstanceType,
		EC2RootVolume: ec2,
	}
	ic.Compute[0].Platform.AWS = &awsinstallertypes.MachinePool{
		InstanceType:  p.workerInstanceType(),
		EC2RootVolume: ec2,
	}

	if len(o.BoundServiceAccountSigningKey) > 0 {
		ic.CredentialsMode = installertypes.ManualCredentialsMode
	}

}

func (p *AWSCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-aws-creds", o.Name)
}

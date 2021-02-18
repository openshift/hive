package clusterresource

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"

	installertypes "github.com/openshift/installer/pkg/types"
	awsinstallertypes "github.com/openshift/installer/pkg/types/aws"
)

const (
	awsInstanceType = "m4.xlarge"
	volumeIOPS      = 100
	volumeSize      = 22
	volumeType      = "gp2"
)

var _ CloudBuilder = (*AWSCloudBuilder)(nil)

// AWSCloudBuilder encapsulates cluster artifact generation logic specific to AWS.
type AWSCloudBuilder struct {
	// AccessKeyID is the AWS access key ID.
	AccessKeyID string
	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string
	// UserTags are user-provided tags to add to resources.
	UserTags map[string]string
	// Region is the AWS region to which to install the cluster
	Region string
}

func NewAWSCloudBuilderFromSecret(credsSecret *corev1.Secret) *AWSCloudBuilder {
	accessKeyID := credsSecret.Data[constants.AWSAccessKeyIDSecretKey]
	secretAccessKey := credsSecret.Data[constants.AWSSecretAccessKeySecretKey]
	return &AWSCloudBuilder{
		AccessKeyID:     string(accessKeyID),
		SecretAccessKey: string(secretAccessKey),
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

func (p *AWSCloudBuilder) generateCloudCertificatesSecret(o *Builder) *corev1.Secret {
	return nil
}

func (p *AWSCloudBuilder) GetCloudPlatform(o *Builder) hivev1.Platform {
	return hivev1.Platform{
		AWS: &hivev1aws.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.CredsSecretName(o),
			},
			Region:   p.Region,
			UserTags: p.UserTags,
		},
	}
}

func (p *AWSCloudBuilder) addMachinePoolPlatform(o *Builder, mp *hivev1.MachinePool) {
	mp.Spec.Platform.AWS = &hivev1aws.MachinePoolPlatform{
		InstanceType: awsInstanceType,
		EC2RootVolume: hivev1aws.EC2RootVolume{
			IOPS: volumeIOPS,
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

	// Used for both control plane and workers.
	mpp := &awsinstallertypes.MachinePool{
		InstanceType: awsInstanceType,
		EC2RootVolume: awsinstallertypes.EC2RootVolume{
			IOPS: volumeIOPS,
			Size: volumeSize,
			Type: volumeType,
		},
	}
	ic.ControlPlane.Platform.AWS = mpp
	ic.Compute[0].Platform.AWS = mpp

}

func (p *AWSCloudBuilder) CredsSecretName(o *Builder) string {
	return fmt.Sprintf("%s-aws-creds", o.Name)
}

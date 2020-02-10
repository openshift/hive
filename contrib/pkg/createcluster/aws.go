package createcluster

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"

	installertypes "github.com/openshift/installer/pkg/types"
	awsinstallertypes "github.com/openshift/installer/pkg/types/aws"
)

const (
	awsRegion       = "us-east-1"
	awsInstanceType = "m4.xlarge"
	volumeIOPS      = 100
	volumeSize      = 22
	volumeType      = "gp2"
)

var _ cloudProvider = (*awsCloudProvider)(nil)

type awsCloudProvider struct {
}

func (p *awsCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	defaultCredsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
	accessKeyID, secretAccessKey, err := awsutils.GetAWSCreds(o.CredsFile, defaultCredsFilePath)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.credsSecretName(o),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}

func (p *awsCloudProvider) addPlatformDetails(
	o *Options,
	cd *hivev1.ClusterDeployment,
	machinePool *hivev1.MachinePool,
	installConfig *installertypes.InstallConfig,
) error {

	// Inject platform details into ClusterDeployment:
	cd.Spec.Platform = hivev1.Platform{
		AWS: &hivev1aws.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
			Region: awsRegion,
		},
	}

	machinePool.Spec.Platform.AWS = &hivev1aws.MachinePoolPlatform{
		InstanceType: awsInstanceType,
		EC2RootVolume: hivev1aws.EC2RootVolume{
			IOPS: volumeIOPS,
			Size: volumeSize,
			Type: volumeType,
		},
	}

	// Inject platform details into InstallConfig:
	installConfig.Platform = installertypes.Platform{
		AWS: &awsinstallertypes.Platform{
			Region: awsRegion,
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
	installConfig.ControlPlane.Platform.AWS = mpp
	installConfig.Compute[0].Platform.AWS = mpp

	return nil
}

func (p *awsCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-aws-creds", o.Name)
}

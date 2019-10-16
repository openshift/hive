package createcluster

import (
	"fmt"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
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

func (p *awsCloudProvider) addPlatformDetails(o *Options, cd *hivev1.ClusterDeployment) error {
	cd.Spec.Platform = hivev1.Platform{
		AWS: &hivev1aws.Platform{
			Region: "us-east-1",
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		AWS: &hivev1aws.PlatformSecrets{
			Credentials: corev1.LocalObjectReference{
				Name: p.credsSecretName(o),
			},
		},
	}
	// Used for both control plane and workers.
	mpp := &hivev1aws.MachinePoolPlatform{
		InstanceType: "m4.large",
		EC2RootVolume: hivev1aws.EC2RootVolume{
			IOPS: 100,
			Size: 22,
			Type: "gp2",
		},
	}

	cd.Spec.ControlPlane.Platform = hivev1.MachinePoolPlatform{
		AWS: mpp,
	}
	for i := range cd.Spec.Compute {
		cd.Spec.Compute[i].Platform = hivev1.MachinePoolPlatform{
			AWS: mpp,
		}
	}
	return nil
}

func (p *awsCloudProvider) credsSecretName(o *Options) string {
	return fmt.Sprintf("%s-aws-creds", o.Name)
}

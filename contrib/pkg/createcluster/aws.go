package createcluster

import (
	"fmt"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
)

var _ cloudProvider = (*awsCloudProvider)(nil)

type awsCloudProvider struct {
}

func (p *awsCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	accessKeyID, secretAccessKey, err := p.getAWSCreds(o)
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

func (p *awsCloudProvider) getAWSCreds(o *Options) (string, string, error) {
	credsFilePath := filepath.Join(o.homeDir, ".aws", "credentials")
	switch {
	case o.CredsFile != "":
		credsFilePath = o.CredsFile
	default:
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
		if len(secretAccessKey) > 0 && len(accessKeyID) > 0 {
			return accessKeyID, secretAccessKey, nil
		}
	}
	credFile, err := ini.Load(credsFilePath)
	if err != nil {
		log.Error("Cannot load AWS credentials")
		return "", "", err
	}
	defaultSection, err := credFile.GetSection("default")
	if err != nil {
		log.Error("Cannot get default section from AWS credentials file")
		return "", "", err
	}
	accessKeyIDValue := defaultSection.Key("aws_access_key_id")
	secretAccessKeyValue := defaultSection.Key("aws_secret_access_key")
	if accessKeyIDValue == nil || secretAccessKeyValue == nil {
		log.Error("AWS credentials file missing keys in default section")
	}
	return accessKeyIDValue.String(), secretAccessKeyValue.String(), nil
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

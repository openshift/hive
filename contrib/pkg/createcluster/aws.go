package createcluster

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ cloudProvider = (*awsCloudProvider)(nil)

type awsCloudProvider struct {
}

func (acp *awsCloudProvider) generateCredentialsSecret(o *Options) (*corev1.Secret, error) {
	accessKeyID, secretAccessKey, err := acp.getAWSCreds(o)
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-aws-creds", o.Name),
			Namespace: o.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}

func (acp *awsCloudProvider) getAWSCreds(o *Options) (string, string, error) {
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	if len(secretAccessKey) > 0 && len(accessKeyID) > 0 {
		return accessKeyID, secretAccessKey, nil
	}
	if len(o.AWSCredsFile) > 0 {
		credFile, err := ini.Load(o.AWSCredsFile)
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
		accessKeyID, secretAccessKey = accessKeyIDValue.String(), secretAccessKeyValue.String()
		return accessKeyID, secretAccessKey, nil
	}
	log.Error("Could not find AWS credentials")
	return "", "", fmt.Errorf("No AWS credentials")
}

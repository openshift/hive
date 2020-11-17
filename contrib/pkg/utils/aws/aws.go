package aws

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetAWSCreds reads AWS credentials either from either the specified credentials file,
// the standard environment variables, or a default credentials file. (~/.aws/credentials)
// The defaultCredsFile will only be used if credsFile is empty and the environment variables
// are not set.
func GetAWSCreds(credsFile, defaultCredsFile string) (string, string, error) {
	credsFilePath := defaultCredsFile
	switch {
	case credsFile != "":
		credsFilePath = credsFile
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

// GenerateAWSCredentialsSecretFromCredsFile will create a Secret containing AWS credentials loaded from
// the given file.
func GenerateAWSCredentialsSecretFromCredsFile(credsFile string) (*corev1.Secret, error) {
	accessKeyID, secretAccessKey, err := GetAWSCreds(credsFile, "")
	if err != nil {
		return nil, err
	}
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("aws-dns-creds-%s", uuid.New().String()[:5]),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"aws_access_key_id":     accessKeyID,
			"aws_secret_access_key": secretAccessKey,
		},
	}, nil
}

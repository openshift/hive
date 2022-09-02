package aws

import (
	"context"
	"os"
	"path/filepath"

	"github.com/openshift/hive/pkg/constants"
	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func Hive1862Experiment(c client.Client) {
	if os.Getenv("HIVE_1862_EXPERIMENT") == "" {
		return
	}
	// Load up the AWS creds from the secret.
	credsNamespace := os.Getenv("AWS_CREDS_SECRET_NAMESPACE")
	if credsNamespace == "" {
		log.Fatal("AWS_CREDS_SECRET_NAMESPACE environment variable is required in scale mode")
	}
	credsName := os.Getenv("AWS_CREDS_SECRET_NAME")
	if credsName == "" {
		log.Fatal("AWS_CREDS_SECRET_NAME environment variable is required in scale mode")
	}
	credsSecret := &corev1.Secret{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: credsNamespace, Name: credsName}, credsSecret); err != nil {
		log.WithError(err).Fatal("Failed to load AWS credentials secret")
	}
	// Should we bounce if any of the following already exist?
	if id := string(credsSecret.Data[constants.AWSAccessKeyIDSecretKey]); id != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", id)
	}
	if secret := string(credsSecret.Data[constants.AWSSecretAccessKeySecretKey]); secret != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secret)
	}
	if config := credsSecret.Data[constants.AWSConfigSecretKey]; len(config) != 0 {
		// Lay this down as a file
		path := filepath.Join(constants.AWSCredsMount, constants.AWSConfigSecretKey)
		if err := os.WriteFile(path, config, 0400); err != nil {
			log.WithError(err).Fatalf("Failed to write AWS config file at %s", path)
		}
		os.Setenv("AWS_CONFIG_FILE", path)
	}
}

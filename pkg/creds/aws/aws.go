package aws

import (
	"errors"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	ini "gopkg.in/ini.v1"

	installertypes "github.com/openshift/installer/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
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

var awsConfigForbidCredentialProcess utils.ProjectToDirFileFilter = func(key string, contents []byte) (basename string, newContents []byte, err error) {
	// First, only process aws_config
	bn, newContents, err := utils.ProjectOnlyTheseKeys(constants.AWSConfigSecretKey)(key, contents)
	// If that passed, scrub for credential_process
	if err == nil && bn != "" && awsclient.ContainsCredentialProcess(newContents) {
		return "", nil, errors.New("credential_process is insecure and thus forbidden")
	}
	return bn, newContents, err
}

// ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures AWS credential environment variables and config files
// accordingly.
func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata) {
	credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME")
	if credsSecret == nil {
		return
	}
	// Should we bounce if any of the following already exist?
	if id := string(credsSecret.Data[constants.AWSAccessKeyIDSecretKey]); id != "" {
		os.Setenv("AWS_ACCESS_KEY_ID", id)
	}
	if secret := string(credsSecret.Data[constants.AWSSecretAccessKeySecretKey]); secret != "" {
		os.Setenv("AWS_SECRET_ACCESS_KEY", secret)
	}
	if config := credsSecret.Data[constants.AWSConfigSecretKey]; len(config) != 0 {
		// Lay this down as a file, but forbid credential_process
		utils.ProjectToDir(credsSecret, constants.AWSCredsMount, awsConfigForbidCredentialProcess)
		os.Setenv("AWS_CONFIG_FILE", filepath.Join(constants.AWSCredsMount, constants.AWSConfigSecretKey))
	}
	// This would normally allow credential_process in the config file, but we checked for that above.
	os.Setenv("AWS_SDK_LOAD_CONFIG", "true")
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

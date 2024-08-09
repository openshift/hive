package azure

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/util/homedir"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads Azure credentials used for install/uninstall from either the default
// credentials file (~/.azure/osServiceAccount.json), the standard environment variable,
// or provided credsFile location (in increasing order of preference).
func GetCreds(credsFile string) ([]byte, error) {
	credsFilePath := filepath.Join(homedir.HomeDir(), ".azure", constants.AzureCredentialsName)
	if l := os.Getenv(constants.AzureCredentialsEnvVar); l != "" {
		credsFilePath = l
	}
	if credsFile != "" {
		credsFilePath = credsFile
	}
	log.WithField("credsFilePath", credsFilePath).Info("Loading azure creds")
	return os.ReadFile(credsFilePath)
}

// ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures Azure credential environment variables and config files
// accordingly.
func ConfigureCreds(c client.Client) {
	credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME")
	if credsSecret == nil {
		return
	}
	utils.ProjectToDir(credsSecret, constants.AzureCredentialsDir, nil)
	os.Setenv(constants.AzureCredentialsEnvVar, constants.AzureCredentialsDir+"/"+constants.AzureCredentialsName)
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

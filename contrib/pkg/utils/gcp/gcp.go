package gcp

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/util/homedir"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads GCP credentials either from either the specified credentials file,
// the standard environment variables, or a default credentials file. (~/.gcp/osServiceAccount.json)
// The defaultCredsFile will only be used if credsFile is empty and the environment variables
// are not set.
func GetCreds(credsFile string) ([]byte, error) {
	credsFilePath := filepath.Join(homedir.HomeDir(), ".gcp", constants.GCPCredentialsName)
	if l := os.Getenv("GOOGLE_CREDENTIALS"); l != "" {
		credsFilePath = l
	}
	if credsFile != "" {
		credsFilePath = credsFile
	}
	log.WithField("credsFilePath", credsFilePath).Info("Loading gcp service account")
	return os.ReadFile(credsFilePath)
}

// ConfigureCreds loads a secret designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures GCP credential environment variables and config files
// accordingly.
func ConfigureCreds(c client.Client) {
	credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME")
	if credsSecret == nil {
		return
	}
	utils.ProjectToDir(credsSecret, constants.GCPCredentialsDir, nil)
	os.Setenv("GOOGLE_CREDENTIALS", constants.GCPCredentialsDir+"/"+constants.GCPCredentialsName)
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

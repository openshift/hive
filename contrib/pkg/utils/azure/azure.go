package azure

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/util/homedir"

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
	return ioutil.ReadFile(credsFilePath)
}

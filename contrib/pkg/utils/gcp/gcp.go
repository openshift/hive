package gcp

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads GCP credentials either from either the specified credentials file,
// the standard environment variables, or a default credentials file. (~/.gcp/osServiceAccount.json)
// The defaultCredsFile will only be used if credsFile is empty and the environment variables
// are not set.
func GetCreds(credsFile string) ([]byte, error) {
	credsFilePath := filepath.Join(os.Getenv("HOME"), ".gcp", constants.GCPCredentialsName)
	if l := os.Getenv("GCP_SHARED_CREDENTIALS_FILE"); l != "" {
		credsFilePath = l
	}
	if credsFile != "" {
		credsFilePath = credsFile
	}
	log.Infof("Loading gcp service account from: %s", credsFilePath)
	return ioutil.ReadFile(credsFilePath)
}

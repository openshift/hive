package gcp

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/util/homedir"

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
	return ioutil.ReadFile(credsFilePath)
}

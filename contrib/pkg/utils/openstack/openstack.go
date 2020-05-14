package openstack

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"k8s.io/client-go/util/homedir"

	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads OpenStack credentials either from the specified credentials file,
// ~/.config/openstack/clouds.yaml, or /etc/openstack/clouds.yaml
func GetCreds(credsFile string) ([]byte, error) {
	if credsFile == "" {
		for _, filePath := range []string{filepath.Join(homedir.HomeDir(), ".config", "openstack", constants.OpenStackCredentialsName),
			"/etc/openstack"} {

			_, err := os.Stat(filePath)
			if err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			if os.IsNotExist(err) {
				continue
			}
			credsFile = filePath
			break
		}
	}
	log.WithField("credsFile", credsFile).Info("Loading OpenStack creds")
	return ioutil.ReadFile(credsFile)
}

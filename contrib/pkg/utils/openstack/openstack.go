package openstack

import (
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads OpenStack credentials either from the specified credentials file,
// ~/.config/openstack/clouds.yaml, or /etc/openstack/clouds.yaml
func GetCreds(credsFile string) ([]byte, error) {
	if credsFile == "" {
		for _, filePath := range []string{filepath.Join(os.Getenv("HOME"), ".config", "openstack", constants.OpenStackCredentialsName),
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
	log.Infof("Loading OpenStack creds from: %s", credsFile)
	return ioutil.ReadFile(credsFile)
}

package ovirt

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"k8s.io/client-go/util/homedir"

	"github.com/openshift/hive/pkg/constants"
)

// GetCreds reads oVirt credentials either from the specified credentials file,
// or ~/.ovirt/ovirt-config.yaml
func GetCreds(credsFile string) ([]byte, error) {
	if credsFile == "" {
		for _, filePath := range []string{filepath.Join(homedir.HomeDir(), ".ovirt", constants.OvirtCredentialsName)} {

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
	return ioutil.ReadFile(credsFile)
}

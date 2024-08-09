package openstack

import (
	"os"
	"path/filepath"

	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/client-go/util/homedir"

	"github.com/openshift/hive/contrib/pkg/utils"
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
	return os.ReadFile(credsFile)
}

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures OpenStack credential config files accordingly.
func ConfigureCreds(c client.Client) {
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		utils.ProjectToDir(credsSecret, constants.OpenStackCredentialsDir, nil)
	}
	if certsSecret := utils.LoadSecretOrDie(c, "CERTS_SECRET_NAME"); certsSecret != nil {
		utils.ProjectToDir(certsSecret, constants.OpenStackCertificatesDir, nil)
		utils.InstallCerts(constants.OpenStackCertificatesDir)
	}
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

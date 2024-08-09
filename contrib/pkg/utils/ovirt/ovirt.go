package ovirt

import (
	"os"
	"path/filepath"

	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/contrib/pkg/utils"
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
	return os.ReadFile(credsFile)
}

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures Ovirt credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client) {
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		utils.ProjectToDir(credsSecret, constants.OvirtCredentialsDir, nil)
		os.Setenv(constants.OvirtConfigEnvVar, constants.OvirtCredentialsDir+"/"+constants.OvirtCredentialsName)
	}
	if certsSecret := utils.LoadSecretOrDie(c, "CERTS_SECRET_NAME"); certsSecret != nil {
		utils.ProjectToDir(certsSecret, constants.OvirtCertificatesDir, nil)
		utils.InstallCerts(constants.OvirtCertificatesDir)
	}
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

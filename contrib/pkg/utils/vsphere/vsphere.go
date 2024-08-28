package vsphere

import (
	"os"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures VSphere credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client) {
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		if username := string(credsSecret.Data[constants.UsernameSecretKey]); username != "" {
			os.Setenv(constants.VSphereUsernameEnvVar, username)
		}
		if password := string(credsSecret.Data[constants.PasswordSecretKey]); password != "" {
			os.Setenv(constants.VSpherePasswordEnvVar, password)
		}
		// NOTE: I think this is only used for terminateWhenFilesChange(), which we don't really
		// care about anymore. Can we get rid of it?
		utils.ProjectToDir(credsSecret, constants.VSphereCredentialsDir, nil)
	}
	if certsSecret := utils.LoadSecretOrDie(c, "CERTS_SECRET_NAME"); certsSecret != nil {
		utils.ProjectToDir(certsSecret, constants.VSphereCertificatesDir, nil)
		utils.InstallCerts(constants.VSphereCertificatesDir)
	}
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

package nutanix

import (
	"os"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"

	"sigs.k8s.io/controller-runtime/pkg/client"

	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/nutanix"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures Nutanix credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata) {
	// Spoof metadata for legacy code paths
	if metadata == nil {
		metadata = &installertypes.ClusterMetadata{
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				Nutanix: &nutanix.Metadata{},
			},
		}
	}
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		if username := string(credsSecret.Data[constants.UsernameSecretKey]); username != "" {
			os.Setenv(constants.NutanixUsernameEnvVar, username)
			metadata.Nutanix.Username = username
		}
		if password := string(credsSecret.Data[constants.PasswordSecretKey]); password != "" {
			os.Setenv(constants.NutanixPasswordEnvVar, password)
			metadata.Nutanix.Password = password
		}

		if certsSecret := utils.LoadSecretOrDie(c, "CERTS_SECRET_NAME"); certsSecret != nil {
			utils.ProjectToDir(certsSecret, constants.NutanixCertificatesDir, nil)
			utils.InstallCerts(constants.NutanixCertificatesDir)
		}
		// Install cluster proxy trusted CA bundle
		utils.InstallCerts(constants.TrustedCABundleDir)
	}
}

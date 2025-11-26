package vsphere

import (
	"os"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"

	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/vsphere"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures VSphere credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata) {
	// Spoof metadata for legacy code paths
	if metadata == nil {
		metadata = &installertypes.ClusterMetadata{
			ClusterPlatformMetadata: installertypes.ClusterPlatformMetadata{
				VSphere: &vsphere.Metadata{},
			},
		}
	}
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		var username, password string
		// Trim whitespace from credentials to handle cross-platform line endings
		if username = strings.TrimSpace(string(credsSecret.Data[constants.UsernameSecretKey])); username != "" {
			os.Setenv(constants.VSphereUsernameEnvVar, username)
		}
		if password = strings.TrimSpace(string(credsSecret.Data[constants.PasswordSecretKey])); password != "" {
			os.Setenv(constants.VSpherePasswordEnvVar, password)
		}
		// Snowflake! We need to (re)inject the creds into the metadata.
		// (They were there originally, but we scrubbed them for security.)
		// If they are unset, the destroyer will fail organically.
		// Accommodate both pre- and post-zonal formats
		if metadata.VSphere.Username != "" {
			metadata.VSphere.Username = username
		}
		if metadata.VSphere.Password != "" {
			metadata.VSphere.Password = password
		}
		for i := range metadata.VSphere.VCenters {
			if metadata.VSphere.VCenters[i].Username != "" {
				metadata.VSphere.VCenters[i].Username = username
			}
			if metadata.VSphere.VCenters[i].Password != "" {
				metadata.VSphere.VCenters[i].Password = password
			}
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

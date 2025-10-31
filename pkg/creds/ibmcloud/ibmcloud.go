package ibmcloud

import (
	"os"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"

	"sigs.k8s.io/controller-runtime/pkg/client"

	installertypes "github.com/openshift/installer/pkg/types"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE
// and CREDS_SECRET_NAME and configures IBMCloud credential environment variables accordingly.
func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata) {
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		if key := string(credsSecret.Data[constants.IBMCloudAPIKeySecretKey]); key != "" {
			os.Setenv(constants.IBMCloudAPIKeyEnvVar, key)
		}
	}
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

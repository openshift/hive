package nutanix

import (
	"os"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures Nutanix credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client) {
	if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
		if username := string(credsSecret.Data[constants.UsernameSecretKey]); username != "" {
			os.Setenv(constants.NutanixUsernameEnvVar, username)
		}
		if password := string(credsSecret.Data[constants.PasswordSecretKey]); password != "" {
			os.Setenv(constants.NutanixPasswordEnvVar, password)
		}
	}
}

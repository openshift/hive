package vsphere

import (
	"strings"

	log "github.com/sirupsen/logrus"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"

	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/vsphere"
)

// ConfigureCreds loads secrets designated by the environment variables CLUSTERDEPLOYMENT_NAMESPACE,
// CREDS_SECRET_NAME, and CERTS_SECRET_NAME and configures VSphere credential environment variables
// and config files accordingly.
func ConfigureCreds(c client.Client, metadata *installertypes.ClusterMetadata) {
	// Snowflake: installmanager doesn't need the creds secret, as the creds are embedded in
	// the install-config. It signifies this by passing a nil metadata.
	if metadata != nil {
		if credsSecret := utils.LoadSecretOrDie(c, "CREDS_SECRET_NAME"); credsSecret != nil {
			if bVCenters := credsSecret.Data["vcenters"]; len(bVCenters) > 0 {
				// Post-zonal: array of vcenters in json format.
				if metadata.VSphere == nil { // should never happen
					metadata.VSphere = &vsphere.Metadata{}
				}
				if err := yaml.Unmarshal(bVCenters, &metadata.VSphere.VCenters); err != nil {
					log.WithError(err).Fatal("failed to unmarshal vcenters from credentials secret")
				}
			} else {
				// Old style creds Secret with flat username/password: copy the creds across all vcenters
				username := strings.TrimSpace(string(credsSecret.Data[constants.UsernameSecretKey]))
				password := strings.TrimSpace(string(credsSecret.Data[constants.PasswordSecretKey]))
				// Caller pre-populated this with just the VCenter names
				for i := range metadata.VSphere.VCenters {
					metadata.VSphere.VCenters[i].Username = username
					metadata.VSphere.VCenters[i].Password = password
				}
			}
		}
	}
	if certsSecret := utils.LoadSecretOrDie(c, "CERTS_SECRET_NAME"); certsSecret != nil {
		utils.ProjectToDir(certsSecret, constants.VSphereCertificatesDir, nil)
		utils.InstallCerts(constants.VSphereCertificatesDir)
	}
	// Install cluster proxy trusted CA bundle
	utils.InstallCerts(constants.TrustedCABundleDir)
}

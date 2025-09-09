package deprovision

import (
	"encoding/json"
	"log"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/installer/pkg/destroy/providers"
	"github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/aws"
	"github.com/openshift/installer/pkg/types/azure"
	"github.com/openshift/installer/pkg/types/gcp"
	"github.com/openshift/installer/pkg/types/ibmcloud"
	"github.com/openshift/installer/pkg/types/nutanix"
	"github.com/openshift/installer/pkg/types/openstack"
	"github.com/openshift/installer/pkg/types/vsphere"

	"github.com/openshift/hive/contrib/pkg/utils"
	awsutil "github.com/openshift/hive/contrib/pkg/utils/aws"
	azureutil "github.com/openshift/hive/contrib/pkg/utils/azure"
	gcputil "github.com/openshift/hive/contrib/pkg/utils/gcp"
	ibmcloudutil "github.com/openshift/hive/contrib/pkg/utils/ibmcloud"
	nutanixutil "github.com/openshift/hive/contrib/pkg/utils/nutanix"
	openstackutil "github.com/openshift/hive/contrib/pkg/utils/openstack"
	vsphereutil "github.com/openshift/hive/contrib/pkg/utils/vsphere"
	"github.com/openshift/hive/pkg/constants"

	"github.com/spf13/cobra"
)

// NewDeprovisionCommand is the entrypoint to create the 'deprovision' subcommand
func NewDeprovisionCommand() *cobra.Command {
	var credsDir string
	var mjSecretName string
	var logLevel string
	cmd := &cobra.Command{
		Use:   "deprovision",
		Short: "Deprovision clusters in supported cloud providers",
		Long: `Platform subcommands use a legacy code path and are deprecated. \
To run the generic destroyer, use the --metadata-json-secret-name parameter.`,
		Run: func(cmd *cobra.Command, args []string) {
			if mjSecretName == "" {
				cmd.Usage()
				return
			}

			// Generic deprovision flow using metadata.json
			logger, err := utils.NewLogger(logLevel)
			if err != nil {
				log.Fatalf("failed to create logger: %s", err)
			}

			c, err := utils.GetClient("hiveutil-deprovision-generic")
			if err != nil {
				logger.WithError(err).Fatal("failed to create kube client")
			}

			// TODO: Refactor LoadSecretOrDie to avoid this setenv/getenv cycle
			k := "METADATA_JSON_SECRET_NAME"
			os.Setenv(k, mjSecretName)
			mjSecret := utils.LoadSecretOrDie(c, k)
			if mjSecret == nil {
				// This should not be reachable -- we should have Fatal()ed in LoadSecretOrDie()
				logger.WithField("secretName", mjSecretName).Fatal("failed to load metadata.json Secret")
			}

			mjBytes, ok := mjSecret.Data[constants.MetadataJSONSecretKey]
			if !ok {
				logger.Fatalf("metadata.json Secret did not contain %q key", constants.MetadataJSONSecretKey)
			}

			var metadata *types.ClusterMetadata
			if err = json.Unmarshal(mjBytes, &metadata); err != nil {
				logger.WithError(err).Fatal("failed to unmarshal metadata.json")
			}

			platform := metadata.Platform()
			if platform == "" {
				logger.Fatal("no platform configured in metadata.json")
			}

			// TODO: Make a registry or interface for this
			var ConfigureCreds func(client.Client)
			switch platform {
			case aws.Name:
				ConfigureCreds = awsutil.ConfigureCreds
			case azure.Name:
				ConfigureCreds = azureutil.ConfigureCreds
			case gcp.Name:
				ConfigureCreds = gcputil.ConfigureCreds
			case ibmcloud.Name:
				ConfigureCreds = ibmcloudutil.ConfigureCreds
			case nutanix.Name:
				// Snowflake! We need to inject the creds into the metadata.
				// If env vars are unset, the destroyer will fail organically.
				ConfigureCreds = func(c client.Client) {
					nutanixutil.ConfigureCreds(c)
					metadata.Nutanix.Username = os.Getenv(constants.NutanixUsernameEnvVar)
					metadata.Nutanix.Password = os.Getenv(constants.NutanixPasswordEnvVar)
				}
			case openstack.Name:
				ConfigureCreds = openstackutil.ConfigureCreds
			case vsphere.Name:
				// Snowflake! We need to (re)inject the creds into the metadata.
				// (They were there originally, but we scrubbed them for security.)
				// If env vars are unset, the destroyer will fail organically.
				ConfigureCreds = func(c client.Client) {
					vsphereutil.ConfigureCreds(c)
					username, password := os.Getenv(constants.VSphereUsernameEnvVar), os.Getenv(constants.VSpherePasswordEnvVar)
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
				}
			}

			ConfigureCreds(c)

			destroyerBuilder, ok := providers.Registry[platform]
			if !ok {
				logger.WithField("platform", platform).Fatal("no destroyers registered for platform")
			}

			destroyer, err := destroyerBuilder(logger, metadata)
			if err != nil {
				logger.WithError(err).Fatal("failed to create destroyer")
			}

			// Ignore quota return
			_, err = destroyer.Run()
			if err != nil {
				logger.WithError(err).Fatal("destroyer returned an error")
			}
		},
	}
	flags := cmd.PersistentFlags()
	// TODO: Unused -- remove from here and generate.go
	flags.StringVar(&credsDir, "creds-dir", "", "directory of the creds. Changes in the creds will cause the program to terminate")
	// TODO: Make this more useful to CLI users by accepting a path to a metadata.json file in the file system
	flags.StringVar(&mjSecretName, "metadata-json-secret-name", "", "name of a Secret in the current namespace containing `metadata.json` from the installer")
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")

	// Legacy destroyers
	cmd.AddCommand(NewDeprovisionAzureCommand(logLevel))
	cmd.AddCommand(NewDeprovisionGCPCommand(logLevel))
	cmd.AddCommand(NewDeprovisionIBMCloudCommand(logLevel))
	cmd.AddCommand(NewDeprovisionOpenStackCommand(logLevel))
	cmd.AddCommand(NewDeprovisionvSphereCommand(logLevel))
	cmd.AddCommand(NewDeprovisionNutanixCommand(logLevel))
	return cmd
}

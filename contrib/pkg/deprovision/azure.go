package deprovision

import (
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/azure"
	"github.com/openshift/installer/pkg/destroy/providers"
	"github.com/openshift/installer/pkg/types"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"

	azureutils "github.com/openshift/hive/contrib/pkg/utils/azure"
)

// AzureOptions is the set of options to deprovision an Azure cluster
type AzureOptions struct {
	logLevel  string
	cloudName string
}

// NewDeprovisionAzureCommand is the entrypoint to create the azure deprovision subcommand
func NewDeprovisionAzureCommand() *cobra.Command {
	opt := &AzureOptions{}
	cmd := &cobra.Command{
		Use:   "azure INFRAID --azure-cloud-name CLOUDNAME",
		Short: "Deprovision Azure assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := validate(); err != nil {
				log.WithError(err).Fatal("Failed validating Azure credentials")
			}
			uninstaller, err := completeAzureUninstaller(opt.logLevel, opt.cloudName, args)
			if err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}

			// ClusterQuota stomped in return
			if _, err := uninstaller.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.cloudName, "azure-cloud-name", installertypesazure.PublicCloud.Name(), "cloudName is the name of the Azure cloud environment used to configure the Azure SDK")
	return cmd
}

func validate() error {
	_, err := azureutils.GetCreds("")
	if err != nil {
		return errors.Wrap(err, "failed to get Azure credentials")
	}

	return nil
}

func completeAzureUninstaller(logLevel, cloudName string, args []string) (providers.Destroyer, error) {

	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return nil, err
	}

	logger := log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	metadata := &types.ClusterMetadata{
		InfraID: args[0],
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			Azure: &installertypesazure.Metadata{
				CloudName: installertypesazure.CloudEnvironment(cloudName),
			},
		},
	}

	return azure.New(logger, metadata)
}

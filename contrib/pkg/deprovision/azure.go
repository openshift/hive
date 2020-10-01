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

type azureParams struct {
	logLevel      string
	resourceGroup string
}

// NewDeprovisionAzureCommand is the entrypoint to create the azure deprovision subcommand
func NewDeprovisionAzureCommand() *cobra.Command {
	var params azureParams

	cmd := &cobra.Command{
		Use:   "azure INFRAID",
		Short: "Deprovision Azure assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := validate(); err != nil {
				log.WithError(err).Fatal("Failed validating Azure credentials")
			}
			uninstaller, err := completeAzureUninstaller(args, params)
			if err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}
			if err := uninstaller.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&params.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	// default of empty string for the resource group means the uninstall will look for a resource group
	// matching the infra id.
	flags.StringVar(&params.resourceGroup, "resource-group", "", "name of Azure resource group (if the installer didn't create one)")
	return cmd
}

func validate() error {
	_, err := azureutils.GetCreds("")
	if err != nil {
		return errors.Wrap(err, "failed to get Azure credentials")
	}

	return nil
}

func completeAzureUninstaller(args []string, params azureParams) (providers.Destroyer, error) {

	// Set log level
	level, err := log.ParseLevel(params.logLevel)
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
				CloudName:         installertypesazure.PublicCloud,
				ResourceGroupName: params.resourceGroup,
			},
		},
	}

	return azure.New(logger, metadata)
}

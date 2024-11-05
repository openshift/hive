package deprovision

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/azure"
	"github.com/openshift/installer/pkg/destroy/providers"
	"github.com/openshift/installer/pkg/types"
	installertypesazure "github.com/openshift/installer/pkg/types/azure"

	"github.com/openshift/hive/contrib/pkg/utils"
	azureutils "github.com/openshift/hive/contrib/pkg/utils/azure"
)

// AzureOptions is the set of options to deprovision an Azure cluster
type AzureOptions struct {
	logLevel                    string
	cloudName                   string
	resourceGroupName           string
	baseDomainResourceGroupName string
}

// NewDeprovisionAzureCommand is the entrypoint to create the azure deprovision subcommand
func NewDeprovisionAzureCommand() *cobra.Command {
	opt := &AzureOptions{}
	cmd := &cobra.Command{
		Use:   "azure INFRAID [--azure-cloud-name CLOUDNAME] [--azure-resource-group-name RG] [--azure-base-domain-resource-group-name BDRG]",
		Short: "Deprovision Azure assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			uninstaller, err := opt.completeAzureUninstaller(args)
			if err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}
			if err := validate(); err != nil {
				log.WithError(err).Fatal("Failed validating Azure credentials")
			}

			// ClusterQuota stomped in return
			if _, err := uninstaller.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.cloudName, "azure-cloud-name", installertypesazure.PublicCloud.Name(), "The name of the Azure cloud environment used to configure the Azure SDK")
	flags.StringVar(&opt.resourceGroupName, "azure-resource-group-name", "", "The name of the custom Azure resource group in which the cluster was created when not using the default installer-created resource group")
	flags.StringVar(&opt.baseDomainResourceGroupName, "azure-base-domain-resource-group-name", "", "The name of the custom Azure resource group in which the cluster's DNS records were created when not using the default installer-created resource group or custom resource group")
	return cmd
}

func validate() error {
	_, err := azureutils.GetCreds("")
	if err != nil {
		return errors.Wrap(err, "failed to get Azure credentials")
	}

	return nil
}

func (opt *AzureOptions) completeAzureUninstaller(args []string) (providers.Destroyer, error) {

	logger, err := utils.NewLogger(opt.logLevel)
	if err != nil {
		return nil, err
	}

	client, err := utils.GetClient("hiveutil-deprovision-azure")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get client")
	}
	azureutils.ConfigureCreds(client)

	metadata := &types.ClusterMetadata{
		InfraID: args[0],
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			Azure: &installertypesazure.Metadata{
				CloudName:                   installertypesazure.CloudEnvironment(opt.cloudName),
				ResourceGroupName:           opt.resourceGroupName,
				BaseDomainResourceGroupName: opt.baseDomainResourceGroupName,
			},
		},
	}

	return azure.New(logger, metadata)
}

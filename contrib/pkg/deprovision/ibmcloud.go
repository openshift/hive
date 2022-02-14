package deprovision

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/installer/pkg/destroy/ibmcloud"
	"github.com/openshift/installer/pkg/types"
	typesibmcloud "github.com/openshift/installer/pkg/types/ibmcloud"
)

// ibmCloudDeprovisionOptions is the set of options to deprovision an IBM Cloud cluster
type ibmCloudDeprovisionOptions struct {
	logLevel          string
	infraID           string
	region            string
	accountID         string
	baseDomain        string
	cisInstanceCRN    string
	resourceGroupName string
	vpc               string
	subnets           []string
}

// NewDeprovisionIBMCloudCommand is the entrypoint to create the IBM Cloud deprovision subcommand
func NewDeprovisionIBMCloudCommand() *cobra.Command {
	opt := &ibmCloudDeprovisionOptions{}
	cmd := &cobra.Command{
		Use:   "ibmcloud INFRAID --region=us-east --account-id=ACCOUNT_ID --base-domain=BASE_DOMAIN --cis-instance-crn=CIS_INSTANCE_CRN",
		Short: "Deprovision IBM Cloud assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("failed to complete options")
			}
			if err := opt.Validate(cmd); err != nil {
				log.WithError(err).Fatal("validation failed")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opt.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")

	// Required flags
	flags.StringVar(&opt.accountID, "account-id", "", "IBM Cloud account ID")
	flags.StringVar(&opt.baseDomain, "base-domain", "", "cluster's base domain")
	flags.StringVar(&opt.cisInstanceCRN, "cis-instance-crn", "", "IBM cloud internet services CRN")
	flags.StringVar(&opt.region, "region", "", "region in which to deprovision cluster")
	flags.StringVar(&opt.resourceGroupName, "resource-group-name", "", "IBM resource group name from user provided VPC")

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *ibmCloudDeprovisionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]
	return nil
}

// Validate ensures that option values make sense
func (o *ibmCloudDeprovisionOptions) Validate(cmd *cobra.Command) error {
	ibmCloudAPIKey := os.Getenv(constants.IBMCloudAPIKeyEnvVar)
	if ibmCloudAPIKey == "" {
		return fmt.Errorf("No %s env var set, cannot proceed", constants.IBMCloudAPIKeyEnvVar)
	}
	if o.region == "" {
		cmd.Usage()
		return fmt.Errorf("No --region provided, cannot proceed")
	}
	if o.accountID == "" {
		cmd.Usage()
		return fmt.Errorf("No --acount-id provided, cannot proceed")
	}
	if o.baseDomain == "" {
		cmd.Usage()
		return fmt.Errorf("No --base-domain provided, cannot proceed")
	}
	if o.cisInstanceCRN == "" {
		cmd.Usage()
		return fmt.Errorf("No --cis-instance-crn provided, cannot proceed")
	}
	if o.resourceGroupName == "" {
		cmd.Usage()
		return fmt.Errorf("No --resource-group-name provided, cannot proceed")
	}
	return nil
}

// Run executes the command
func (o *ibmCloudDeprovisionOptions) Run() error {
	// Set log level
	level, err := log.ParseLevel(o.logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
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
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			IBMCloud: &typesibmcloud.Metadata{
				AccountID:         o.accountID,
				BaseDomain:        o.baseDomain,
				CISInstanceCRN:    o.cisInstanceCRN,
				Region:            o.region,
				ResourceGroupName: o.resourceGroupName,
			},
		},
	}

	destroyer, err := ibmcloud.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

package deprovision

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/ibmclient"
	"github.com/openshift/installer/pkg/destroy/ibmcloud"
	"github.com/openshift/installer/pkg/types"
	typesibmcloud "github.com/openshift/installer/pkg/types/ibmcloud"
)

// ibmCloudDeprovisionOptions is the set of options to deprovision an IBM Cloud cluster
type ibmCloudDeprovisionOptions struct {
	accountID         string
	baseDomain        string
	cisInstanceCRN    string
	clusterName       string
	infraID           string
	logLevel          string
	region            string
	resourceGroupName string
	subnets           []string
	vpc               string
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
	flags.StringVar(&opt.baseDomain, "base-domain", "", "cluster's base domain")
	flags.StringVar(&opt.clusterName, "cluster-name", "", "cluster's name")
	flags.StringVar(&opt.region, "region", "", "region in which to deprovision cluster")
	flags.StringVar(&opt.resourceGroupName, "resource-group-name", "", "IBM resource group name from user provided VPC")

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *ibmCloudDeprovisionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	// Create IBMCloud Client
	ibmCloudAPIKey := os.Getenv(constants.IBMCloudAPIKeyEnvVar)
	if ibmCloudAPIKey == "" {
		return fmt.Errorf("No %s env var set, cannot proceed", constants.IBMCloudAPIKeyEnvVar)
	}
	ibmClient, err := ibmclient.NewClient(ibmCloudAPIKey)
	if err != nil {
		return errors.Wrap(err, "Unable to create IBM Cloud client")
	}

	// Retrieve CISInstanceCRN
	cisInstanceCRN, err := ibmclient.GetCISInstanceCRN(ibmClient, context.TODO(), o.baseDomain)
	if err != nil {
		return err
	}
	o.cisInstanceCRN = cisInstanceCRN

	// Retrieve AccountID
	accountID, err := ibmclient.GetAccountID(ibmClient, context.TODO())
	if err != nil {
		return err
	}
	o.accountID = accountID

	return nil
}

// Validate ensures that option values make sense
func (o *ibmCloudDeprovisionOptions) Validate(cmd *cobra.Command) error {
	if o.region == "" {
		cmd.Usage()
		return fmt.Errorf("No --region provided, cannot proceed")
	}
	if o.baseDomain == "" {
		cmd.Usage()
		return fmt.Errorf("No --base-domain provided, cannot proceed")
	}
	if o.resourceGroupName == "" {
		cmd.Usage()
		return fmt.Errorf("No --resource-group-name provided, cannot proceed")
	}
	if o.clusterName == "" {
		cmd.Usage()
		return fmt.Errorf("No --cluster-name provided, cannot proceed")
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
		ClusterName: o.clusterName,
		InfraID:     o.infraID,
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

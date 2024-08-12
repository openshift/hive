package deprovision

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	ibmutils "github.com/openshift/hive/contrib/pkg/utils/ibmcloud"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/ibmclient"
	"github.com/openshift/installer/pkg/destroy/ibmcloud"
	"github.com/openshift/installer/pkg/types"
	typesibmcloud "github.com/openshift/installer/pkg/types/ibmcloud"
)

// ibmCloudDeprovisionOptions is the set of options to deprovision an IBM Cloud cluster
type ibmCloudDeprovisionOptions struct {
	accountID      string
	baseDomain     string
	cisInstanceCRN string
	clusterName    string
	infraID        string
	logLevel       string
	region         string
}

// NewDeprovisionIBMCloudCommand is the entrypoint to create the IBM Cloud deprovision subcommand
func NewDeprovisionIBMCloudCommand() *cobra.Command {
	opt := &ibmCloudDeprovisionOptions{}
	cmd := &cobra.Command{
		Use:   "ibmcloud INFRAID --region=us-east --base-domain=BASE_DOMAIN --cluster-name=CLUSTERNAME",
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

	return cmd
}

// Complete finishes parsing arguments for the command
func (o *ibmCloudDeprovisionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-ibmcloud")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	ibmutils.ConfigureCreds(client)

	// Create IBMCloud Client
	ibmCloudAPIKey := os.Getenv(constants.IBMCloudAPIKeyEnvVar)
	if ibmCloudAPIKey == "" {
		return fmt.Errorf("no %s env var set, cannot proceed", constants.IBMCloudAPIKeyEnvVar)
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
		return fmt.Errorf("no --region provided, cannot proceed")
	}
	if o.baseDomain == "" {
		cmd.Usage()
		return fmt.Errorf("no --base-domain provided, cannot proceed")
	}
	if o.clusterName == "" {
		cmd.Usage()
		return fmt.Errorf("no --cluster-name provided, cannot proceed")
	}
	return nil
}

// Run executes the command
func (o *ibmCloudDeprovisionOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		ClusterName: o.clusterName,
		InfraID:     o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			IBMCloud: &typesibmcloud.Metadata{
				AccountID:         o.accountID,
				BaseDomain:        o.baseDomain,
				CISInstanceCRN:    o.cisInstanceCRN,
				Region:            o.region,
				ResourceGroupName: o.infraID,
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

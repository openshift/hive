package deprovision

import (
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/gcp"
	"github.com/openshift/installer/pkg/types"
	typesgcp "github.com/openshift/installer/pkg/types/gcp"

	"github.com/openshift/hive/contrib/pkg/utils"
	gcputils "github.com/openshift/hive/contrib/pkg/utils/gcp"
	"github.com/openshift/hive/pkg/gcpclient"
)

// gcpOptions is the set of options to deprovision a GCP cluster
type gcpOptions struct {
	logLevel         string
	infraID          string
	region           string
	projectID        string
	networkProjectID string
}

// NewDeprovisionGCPCommand is the entrypoint to create the GCP deprovision subcommand
func NewDeprovisionGCPCommand() *cobra.Command {
	opt := &gcpOptions{}
	cmd := &cobra.Command{
		Use:   "gcp INFRAID --region=REGION",
		Short: "Deprovision GCP assets (as created by openshift-installer)",
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
	flags.StringVar(&opt.region, "region", "", "GCP region where the cluster is installed")
	flags.StringVar(&opt.networkProjectID, "network-project-id", "", "For shared VPC setups")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *gcpOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-gcp")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	gcputils.ConfigureCreds(client)

	return nil
}

// Validate ensures that option values make sense
func (o *gcpOptions) Validate(cmd *cobra.Command) error {
	if o.region == "" {
		cmd.Usage()
		log.Info("Region is required")
		return fmt.Errorf("missing region")
	}

	creds, err := gcputils.GetCreds("")
	if err != nil {
		return errors.Wrap(err, "failed to get GCP credentials")
	}
	projectID, err := gcpclient.ProjectID(creds)
	if err != nil {
		return errors.Wrap(err, "could not get GCP project ID")
	}
	o.projectID = projectID
	return nil
}

// Run executes the command
func (o *gcpOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			GCP: &typesgcp.Metadata{
				Region:           o.region,
				ProjectID:        o.projectID,
				NetworkProjectID: o.networkProjectID,
			},
		},
	}

	destroyer, err := gcp.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

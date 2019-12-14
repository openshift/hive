package deprovision

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/gcp"
	"github.com/openshift/installer/pkg/types"
	typesgcp "github.com/openshift/installer/pkg/types/gcp"

	"github.com/openshift/hive/pkg/gcpclient"
)

// gcpOptions is the set of options to deprovision a GCP cluster
type gcpOptions struct {
	logLevel  string
	infraID   string
	region    string
	projectID string
}

// NewDeprovisionGCPCommand is the entrypoint to create the GCP deprovision subcommand
func NewDeprovisionGCPCommand() *cobra.Command {
	opt := &gcpOptions{}
	cmd := &cobra.Command{
		Use:   "gcp INFRAID --region=REGION --gcp-project-id=GCP_PROJECT_ID",
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
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *gcpOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]
	return nil
}

// Validate ensures that option values make sense
func (o *gcpOptions) Validate(cmd *cobra.Command) error {
	if o.region == "" {
		cmd.Usage()
		log.Info("Region is required")
		return fmt.Errorf("missing region")
	}
	credsFile := os.Getenv("GOOGLE_CREDENTIALS")
	projectID, err := gcpclient.ProjectIDFromFile(credsFile)
	if err != nil {
		return errors.Wrap(err, "could not get GCP project ID")
	}
	o.projectID = projectID
	return nil
}

// Run executes the command
func (o *gcpOptions) Run() error {
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
			GCP: &typesgcp.Metadata{
				Region:    o.region,
				ProjectID: o.projectID,
			},
		},
	}

	destroyer, err := gcp.New(logger, metadata)
	if err != nil {
		return err
	}

	return destroyer.Run()
}

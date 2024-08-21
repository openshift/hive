package deprovision

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	ovirtutils "github.com/openshift/hive/contrib/pkg/utils/ovirt"
	"github.com/openshift/installer/pkg/destroy/ovirt"
	"github.com/openshift/installer/pkg/types"
	typesovirt "github.com/openshift/installer/pkg/types/ovirt"
)

// oVirtOptions is the set of options to deprovision an oVirt cluster
type oVirtOptions struct {
	logLevel  string
	infraID   string
	clusterID string
}

// NewDeprovisionOvirtCommand is the entrypoint to create the oVirt deprovision subcommand
func NewDeprovisionOvirtCommand() *cobra.Command {
	opt := &oVirtOptions{}
	cmd := &cobra.Command{
		Use:   "ovirt INFRAID",
		Short: "Deprovision oVirt assets (as created by openshift-installer)",
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
	flags.StringVar(&opt.clusterID, "ovirt-cluster-id", "", "oVirt cluster ID")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *oVirtOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-ovirt")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	ovirtutils.ConfigureCreds(client)

	return nil
}

// Validate ensures that option values make sense
func (o *oVirtOptions) Validate(cmd *cobra.Command) error {
	if o.clusterID == "" {
		return errors.New("must provide --ovirt-cluster-id or set")
	}
	return nil
}

// Run executes the command
func (o *oVirtOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			Ovirt: &typesovirt.Metadata{
				ClusterID:      o.clusterID,
				RemoveTemplate: true,
			},
		},
	}

	destroyer, err := ovirt.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

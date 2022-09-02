package deprovision

import (
	"fmt"

	"github.com/openshift/hive/contrib/pkg/utils"
	aliutils "github.com/openshift/hive/contrib/pkg/utils/alibabacloud"
	"github.com/openshift/installer/pkg/destroy/alibabacloud"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/types"
	typesalibabacloud "github.com/openshift/installer/pkg/types/alibabacloud"
)

// alibabaCloudDeprovisionOptions is the set of options to deprovision an Alibaba Cloud cluster
type alibabaCloudDeprovisionOptions struct {
	logLevel    string
	clusterName string
	baseDomain  string
	infraID     string
	region      string
}

// NewDeprovisionAlibabaCloudCommand is the entrypoint to create the Alibaba Cloud deprovision subcommand
func NewDeprovisionAlibabaCloudCommand() *cobra.Command {
	opt := &alibabaCloudDeprovisionOptions{}
	cmd := &cobra.Command{
		Use:   "alibabacloud INFRAID --region=cn-hangzhou --base-domain=BASE_DOMAIN --cluster-name=CLUSTERNAME",
		Short: "Deprovision Alibaba Cloud assets (as created by openshift-installer)",
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
func (o *alibabaCloudDeprovisionOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]
	client, err := utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	aliutils.ConfigureCreds(client)
	return nil
}

// Validate ensures that option values make sense
func (o *alibabaCloudDeprovisionOptions) Validate(cmd *cobra.Command) error {
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
func (o *alibabaCloudDeprovisionOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			AlibabaCloud: &typesalibabacloud.Metadata{
				ClusterDomain: fmt.Sprintf("%s.%s", o.clusterName, o.baseDomain),
				Region:        o.region,
			},
		},
	}

	destroyer, err := alibabacloud.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

package deprovision

import (
	"fmt"
	"github.com/openshift/installer/pkg/destroy/nutanix"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/types"
	typesnutanix "github.com/openshift/installer/pkg/types/nutanix"

	"github.com/openshift/hive/contrib/pkg/utils"
	nutanixutils "github.com/openshift/hive/contrib/pkg/utils/nutanix"
	"github.com/openshift/hive/pkg/constants"
)

// nutanixOptions
type nutanixOptions struct {
	logLevel string
	infraID  string
	endpoint string
	port     string
	username string
	password string
}

// NewDeprovisionNutanixCommand
func NewDeprovisionNutanixCommand() *cobra.Command {
	opt := &nutanixOptions{}
	cmd := &cobra.Command{
		Use:   "nutanix INFRAID",
		Short: "Deprovision Nutanix assets (as created by openshift-installer)",
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
	flags.StringVar(&opt.endpoint, "nutanix-endpoint", "", "Domain name or IP address of the Nutanix Prism Central endpoint")
	flags.StringVar(&opt.port, "nutanix-port", "", "Port of the Nutanix Prism Central endpoint")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *nutanixOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-nutanix")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	nutanixutils.ConfigureCreds(client)

	return nil
}

// Validate ensures that option values make sense
func (o *nutanixOptions) Validate(cmd *cobra.Command) error {
	if o.endpoint == "" {
		o.endpoint = os.Getenv(constants.NutanixPrismCentralEndpointEnvVar)
		if o.endpoint == "" {
			return fmt.Errorf("must provide --nutanix-prism-central-endpoint or set %s env var", constants.NutanixPrismCentralEndpointEnvVar)
		}
	}
	o.port = os.Getenv(constants.NutanixPrismCentralPortEnvVar)
	if o.port == "" {
		return fmt.Errorf("must provide --nutanix-prism-central-port or set %s env var", constants.NutanixPrismCentralPortEnvVar)
	}
	o.username = os.Getenv(constants.NutanixUsernameEnvVar)
	if o.username == "" {
		return fmt.Errorf("no %s env var set, cannot proceed", constants.NutanixUsernameEnvVar)
	}
	o.password = os.Getenv(constants.NutanixPasswordEnvVar)
	if o.password == "" {
		return fmt.Errorf("no %s env var set, cannot proceed", constants.NutanixPasswordEnvVar)
	}
	return nil
}

// Run executes the command
func (o *nutanixOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			Nutanix: &typesnutanix.Metadata{
				PrismCentral: o.endpoint,
				Username:     o.username,
				Password:     o.password,
				Port:         o.port,
			},
		},
	}

	destroyer, err := nutanix.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

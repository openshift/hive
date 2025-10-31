package deprovision

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	nutanixutils "github.com/openshift/hive/contrib/pkg/utils/nutanix"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/installer/pkg/destroy/nutanix"
	"github.com/openshift/installer/pkg/types"
	typesnutanix "github.com/openshift/installer/pkg/types/nutanix"
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

func NewDeprovisionNutanixCommand(logLevel string) *cobra.Command {
	opt := &nutanixOptions{
		logLevel: logLevel,
	}

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
	flags.StringVar(&opt.endpoint, constants.CliNutanixPcAddressOpt, "", "Domain name or IP address of the Nutanix Prism Central endpoint")
	flags.StringVar(&opt.port, constants.CliNutanixPcPortOpt, "", "Port of the Nutanix Prism Central endpoint")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *nutanixOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-nutanix")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	nutanixutils.ConfigureCreds(client, nil)

	return nil
}

// Validate ensures that option values make sense
func (o *nutanixOptions) Validate(cmd *cobra.Command) error {
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

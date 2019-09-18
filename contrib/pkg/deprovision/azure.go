package deprovision

import (
	azuresession "github.com/openshift/installer/pkg/asset/installconfig/azure"
	"github.com/openshift/installer/pkg/destroy/azure"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"os"
)

// NewDeprovisionAzureCommand is the entrypoint to create the azure deprovision subcommand
func NewDeprovisionAzureCommand() *cobra.Command {
	opt := &azure.ClusterUninstaller{}
	var logLevel string
	cmd := &cobra.Command{
		Use:   "azure INFRAID",
		Short: "Deprovision Azure assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := completeAzureUninstaller(opt, logLevel, args); err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	return cmd
}

func completeAzureUninstaller(o *azure.ClusterUninstaller, logLevel string, args []string) error {

	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	o.Logger = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	session, err := azuresession.GetSession()
	if err != nil {
		return err
	}

	o.InfraID = args[0]
	o.SubscriptionID = session.Credentials.SubscriptionID
	o.TenantID = session.Credentials.TenantID
	o.GraphAuthorizer = session.GraphAuthorizer
	o.Authorizer = session.Authorizer

	return nil
}

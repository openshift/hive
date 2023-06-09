package awsprivatelink

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var logLevelDebug bool

func NewAWSPrivateLinkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "awsprivatelink",
		Short: "AWS PrivateLink setup and tear down",
		Long: `AWS PrivateLink setup and tear down.
All subcommands require an active cluster.
`,
		PersistentPreRun: setLogLevel,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Usage()
		},
	}

	cmd.AddCommand(NewEnableAWSPrivateLinkCommand())
	cmd.AddCommand(NewDisableAWSPrivateLinkCommand())
	cmd.AddCommand(NewEndpointVPCCommand())

	cmd.PersistentFlags().BoolVarP(&logLevelDebug, "debug", "d", false, "Enable debug level logging")
	return cmd
}

func setLogLevel(cmd *cobra.Command, args []string) {
	switch logLevelDebug {
	case true:
		log.SetLevel(log.DebugLevel)
		log.Debug("Setting log level to debug")
	default:
		log.SetLevel(log.InfoLevel)
	}
}

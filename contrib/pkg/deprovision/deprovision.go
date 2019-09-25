package deprovision

import (
	"github.com/spf13/cobra"
)

// NewDeprovisionCommand is the entrypoint to create the 'deprovision' subcommand
func NewDeprovisionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deprovision",
		Short: "Deprovision clusters in supported cloud providers",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewDeprovisionAzureCommand())
	cmd.AddCommand(NewDeprovisionGCPCommand())
	return cmd
}

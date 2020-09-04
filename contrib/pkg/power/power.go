package power

import (
	"github.com/spf13/cobra"
)

// NewPowerCommand is the entrypoint to create the 'power' subcommand
func NewPowerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "power",
		Short: "Set cluster power state",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewPowerOnCommand())
	cmd.AddCommand(NewPowerOffCommand())
	return cmd
}

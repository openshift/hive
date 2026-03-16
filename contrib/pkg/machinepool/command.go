package machinepool

import "github.com/spf13/cobra"

// NewMachinePoolCommand is the entrypoint for the 'machinepool' subcommand.
func NewMachinePoolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machinepool",
		Short: "Manage MachinePools for ClusterDeployments",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewCreateCommand())
	cmd.AddCommand(NewDetachCommand())
	cmd.AddCommand(NewAdoptCommand())
	return cmd
}

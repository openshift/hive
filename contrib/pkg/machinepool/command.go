package machinepool

import "github.com/spf13/cobra"

func NewMachinePoolCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "machinepool",
		Short: "Utilities to manage MachinePools",
		Long:  "Utilities to manage Hive MachinePools: create new MachinePools, adopt existing MachineSets into management, or detach MachineSets from management",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewAdoptCommand())
	cmd.AddCommand(NewCreateCommand())
	cmd.AddCommand(NewDetachCommand())
	return cmd
}

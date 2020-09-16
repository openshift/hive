package clusterpool

import "github.com/spf13/cobra"

// NewClusterPoolCommand is the entrypoint to create the 'clusterpool' subcommand
func NewClusterPoolCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "clusterpool",
		Short: "Utility to manage ClusterPools",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewCreateClusterPoolCommand())
	cmd.AddCommand(NewClaimClusterPoolCommand())
	return cmd

}

package managedns

import (
	"github.com/spf13/cobra"
)

// NewManageDNSCommand is the entrypoint to create the 'manage-dns' subcommand
func NewManageDNSCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manage-dns",
		Short: "Administer the global managed DNS HiveConfig functionality",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewEnableManageDNSCommand())
	return cmd
}

package adm

import (
	"github.com/openshift/hive/contrib/pkg/adm/managedns"
	"github.com/spf13/cobra"
)

// NewAdmCommand is the entrypoint to create the 'adm' subcommand
func NewAdmCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adm",
		Short: "Hive administration sub-commands",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(managedns.NewManageDNSCommand())
	return cmd
}

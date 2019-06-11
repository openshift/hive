package report

import (
	"github.com/spf13/cobra"
)

// NewClusterReportCommand creates a command that generates and outputs the cluster report.
func NewClusterReportCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Execute reports on the data in a Hive cluster.",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewProvisioningReportCommand())
	cmd.AddCommand(NewDeprovisioningReportCommand())
	return cmd
}

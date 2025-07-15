package deprovision

import (
	"github.com/spf13/cobra"
)

// NewDeprovisionCommand is the entrypoint to create the 'deprovision' subcommand
func NewDeprovisionCommand() *cobra.Command {
	var credsDir string
	cmd := &cobra.Command{
		Use:   "deprovision",
		Short: "Deprovision clusters in supported cloud providers",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVar(&credsDir, "creds-dir", "", "directory of the creds. Changes in the creds will cause the program to terminate")
	cmd.AddCommand(NewDeprovisionAzureCommand())
	cmd.AddCommand(NewDeprovisionGCPCommand())
	cmd.AddCommand(NewDeprovisionIBMCloudCommand())
	cmd.AddCommand(NewDeprovisionOpenStackCommand())
	cmd.AddCommand(NewDeprovisionvSphereCommand())
	cmd.AddCommand(NewDeprovisionOvirtCommand())
	cmd.AddCommand(NewDeprovisionNutanixCommand())
	return cmd
}

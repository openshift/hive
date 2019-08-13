package certificate

import (
	"github.com/spf13/cobra"
)

// NewCertificateCommand returns a utility command to help create a certificate for
// OpenShift clusters on AWS
func NewCertificateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "certificate",
		Short: "Utility to create serving certificates for clusters on AWS with Letsencrypt",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewCreateCertifcateCommand())
	cmd.AddCommand(NewAuthHookCommand())
	cmd.AddCommand(NewCleanupHookCommand())
	return cmd
}

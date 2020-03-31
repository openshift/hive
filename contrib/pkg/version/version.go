package version

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	pkgversion "github.com/openshift/hive/pkg/version"
)

// NewVersionCommand creates a command that generates and outputs the cluster report.
func NewVersionCommand() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Prints version information for the command",
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			log.Infof("%s @ %s", pkgversion.String, pkgversion.Commit)
		},
	}
	return cmd
}

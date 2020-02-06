package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/adm"
	"github.com/openshift/hive/contrib/pkg/certificate"
	"github.com/openshift/hive/contrib/pkg/createcluster"
	"github.com/openshift/hive/contrib/pkg/deprovision"
	"github.com/openshift/hive/contrib/pkg/report"
	"github.com/openshift/hive/contrib/pkg/testresource"
	"github.com/openshift/hive/contrib/pkg/v1migration"
	"github.com/openshift/hive/contrib/pkg/verification"
	"github.com/openshift/hive/pkg/imageset"
	"github.com/openshift/hive/pkg/installmanager"
)

func main() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)

	cmd := newHiveutilCommand()

	err := cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error occurred: %v\n", err)
		os.Exit(1)
	}
}

func newHiveutilCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hiveutil SUB-COMMAND",
		Short: "Utilities for hive",
		Long:  "Contains various utilities for running and testing hive",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(deprovision.NewDeprovisionAWSWithTagsCommand())
	cmd.AddCommand(deprovision.NewDeprovisionCommand())
	cmd.AddCommand(verification.NewVerifyImportsCommand())
	cmd.AddCommand(installmanager.NewInstallManagerCommand())
	cmd.AddCommand(imageset.NewUpdateInstallerImageCommand())
	cmd.AddCommand(testresource.NewTestResourceCommand())
	cmd.AddCommand(createcluster.NewCreateClusterCommand())
	cmd.AddCommand(report.NewClusterReportCommand())
	cmd.AddCommand(v1migration.NewV1MigrationCommand())
	cmd.AddCommand(certificate.NewCertificateCommand())
	cmd.AddCommand(adm.NewAdmCommand())

	return cmd
}

package v1migration

import (
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// NewV1MigrationCommand creates a command that executes the migration utilities.
func NewV1MigrationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "v1migration",
		Short: "Save and restore owner references to Hive resources",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Usage()
		},
	}
	cmd.AddCommand(NewSaveOwnerRefsCommand())
	cmd.AddCommand(NewRestoreOwnerRefsCommand())
	cmd.AddCommand(NewDeleteObjectsCommand())
	cmd.AddCommand(NewRecreateObjectsCommand())
	return cmd
}

func validateWorkDir(workDir string) error {
	switch info, err := os.Stat(workDir); {
	case os.IsNotExist(err):
		return errors.New("work directory does not exist")
	case info == nil:
		return errors.New("could not use work directory")
	case !info.IsDir():
		return errors.New("work directory is not a directory")
	}
	return nil
}

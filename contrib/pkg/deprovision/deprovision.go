package deprovision

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/library-go/pkg/controller/fileobserver"
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if credsDir == "" {
				return
			}
			go terminateWhenFilesChange(credsDir)
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVar(&credsDir, "creds-dir", "", "directory of the creds. Changes in the creds will cause the program to terminate")
	cmd.AddCommand(NewDeprovisionAzureCommand())
	cmd.AddCommand(NewDeprovisionGCPCommand())
	cmd.AddCommand(NewDeprovisionIBMCloudCommand())
	cmd.AddCommand(NewDeprovisionOpenStackCommand())
	cmd.AddCommand(NewDeprovisionvSphereCommand())
	cmd.AddCommand(NewDeprovisionNutanixCommand())
	cmd.AddCommand(NewDeprovisionOvirtCommand())
	return cmd
}

func terminateWhenFilesChange(dir string) {
	fileContents := map[string][]byte{}
	var filenames []string
	if err := filepath.Walk(
		dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return errors.Wrapf(err, "problem encountered walking %s", path)
			}
			if info.IsDir() {
				// skip directories
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				// skip symlinks
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return errors.Wrapf(err, "could not read contents of %s", path)
			}
			fileContents[path] = data
			filenames = append(filenames, path)
			return nil
		},
	); err != nil {
		log.WithError(err).Fatal("could not read files for creds watch")
	}
	obs, err := fileobserver.NewObserver(10 * time.Second)
	if err != nil {
		log.WithError(err).Fatal("could not set up file observer for creds watch")
	}
	obs.AddReactor(
		func(filename string, action fileobserver.ActionType) error {
			log.WithField("filename", filename).Fatal("exiting because creds file changed")
			return nil
		},
		fileContents,
		filenames...,
	)
	stop := make(chan struct{})
	go func() {
		log.WithField("files", filenames).Info("running file observer")
		obs.Run(stop)
		log.Fatal("file observer stopped")
	}()
}

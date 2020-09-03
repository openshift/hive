package deprovision

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/spf13/cobra"

	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/constants"
)

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
// TODO: Port to a sub-command of deprovision.
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &aws.ClusterUninstaller{}
	var completionFile string
	var logLevel string
	cmd := &cobra.Command{
		Use:   "aws-tag-deprovision KEY=VALUE ...",
		Short: "Deprovision AWS assets (as created by openshift-installer) with the given tag(s)",
		Long:  "Deprovision AWS assets (as created by openshift-installer) with the given tag(s).  A resource matches the filter if any of the key/value pairs are in its tags.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := completeAWSUninstaller(opt, logLevel, args); err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}

			if _, err := os.Stat(filepath.Dir(constants.DeprovisionCompletedFile)); os.IsNotExist(err) {
				err := os.MkdirAll(filepath.Dir(constants.DeprovisionCompletedFile), os.ModePerm)
				if err != nil {
					log.WithError(err).Fatal("error creating dir for completion file")
				}
			}

			if completionFile != "" {
				log.Infof("Writing file to signal completion: %s", constants.DeprovisionCompletedFile)
				err := ioutil.WriteFile(constants.DeprovisionCompletedFile, []byte("0"), 0644)
				if err != nil {
					log.WithError(err).Fatal("error writing completion file")
				}
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region to use")
	// For use by sidecar container.
	flags.StringVar(&completionFile, "completion-file", "", "file to write when command is complete")
	return cmd
}

func completeAWSUninstaller(o *aws.ClusterUninstaller, logLevel string, args []string) error {
	for _, arg := range args {
		filter := aws.Filter{}
		err := parseFilter(filter, arg)
		if err != nil {
			return fmt.Errorf("cannot parse filter %s: %v", arg, err)
		}
		o.Filters = append(o.Filters, filter)
	}

	// Set log level
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	o.Logger = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	return nil
}

func parseFilter(filterMap aws.Filter, str string) error {
	parts := strings.SplitN(str, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("incorrectly formatted filter")
	}

	filterMap[parts[0]] = parts[1]

	return nil
}

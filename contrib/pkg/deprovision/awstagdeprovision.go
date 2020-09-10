package deprovision

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/openshift/library-go/pkg/controller/fileobserver"

	"github.com/openshift/hive/pkg/constants"
)

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
// TODO: Port to a sub-command of deprovision.
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &aws.ClusterUninstaller{}
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

			// If environment variables are not set, assume we're running in a deprovision pod with an aws credentials secret mounted.
			// Parse credentials from files mounted in as a secret volume and set as env vars. We use the file
			// instead of passing env vars directly so we can monitor for changes and restart the pod if an admin
			// modifies the creds secret after the uninstall job has launched.
			if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
				log.WithField("credsMount", constants.AWSCredsMount).Info(
					"AWS environment variables not set, assume running in pod with cred secret mounted")
				awsAccessKeyIDFile := filepath.Join(constants.AWSCredsMount, constants.AWSAccessKeyIDSecretKey)
				awsSecretAccessKeyFile := filepath.Join(constants.AWSCredsMount, constants.AWSSecretAccessKeySecretKey)
				files := make(map[string][]byte, 2)

				data, err := ioutil.ReadFile(awsAccessKeyIDFile)
				if err != nil {
					log.WithError(err).Fatalf("error reading %s file", awsAccessKeyIDFile)
				}
				os.Setenv("AWS_ACCESS_KEY_ID", string(data))
				files[awsAccessKeyIDFile] = data

				data, err = ioutil.ReadFile(awsSecretAccessKeyFile)
				if err != nil {
					log.WithError(err).Fatalf("error reading %s file", awsSecretAccessKeyFile)
				}
				os.Setenv("AWS_SECRET_ACCESS_KEY", string(data))
				files[awsSecretAccessKeyFile] = data

				fullFilenames := []string{
					awsAccessKeyIDFile,
					awsSecretAccessKeyFile,
				}
				obs, err := fileobserver.NewObserver(10 * time.Second)
				if err != nil {
					log.WithError(err).Fatal("could not set up file observer to check for credentials changes")
				}
				obs.AddReactor(func(filename string, action fileobserver.ActionType) error {
					log.WithField("filename", filename).Info("exiting because credentials secret mount file changed")
					os.Exit(1)
					return nil
				}, files, fullFilenames...)

				stop := make(chan struct{})
				go func() {
					log.WithField("files", fullFilenames).Info("running file observer")
					obs.Run(stop)
					log.Fatal("file observer stopped")
				}()
			}

			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region to use")
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

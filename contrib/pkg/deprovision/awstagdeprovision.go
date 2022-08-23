package deprovision

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	awsutils "github.com/openshift/hive/contrib/pkg/utils/aws"
	"github.com/openshift/installer/pkg/destroy/aws"
)

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
// TODO: Port to a sub-command of deprovision.
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &aws.ClusterUninstaller{}
	var credsDir string
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

			if credsDir != "" {
				go terminateWhenFilesChange(credsDir)
			}

			// ClusterQuota stomped in return
			if _, err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region to use")
	// TODO: This isn't actually informing us where to get the creds. Its only effect is to terminate the command if
	// whatever is in here changes -- whether or not it's actually what the command is using. It happens to match
	// what we supply via the AWS_CONFIG_FILE env var when we generate the uninstaller job for deprovisioning. But
	// a) that job is running in a pod where the creds file should never change, so it's not much good in that case; and
	// b) it would not have the expected effect if used in a standalone invocation.
	// So: can we just get rid of this arg and the terminateWhenFilesChange goroutine?
	flags.StringVar(&credsDir, "creds-dir", "", "directory of the creds. Changes in the creds will cause the program to terminate")
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

	var err error
	if o.Logger, err = utils.NewLogger(logLevel); err != nil {
		return err
	}

	client, err := utils.GetClient()
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	awsutils.ConfigureCreds(client)

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

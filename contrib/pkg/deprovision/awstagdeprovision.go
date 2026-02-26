package deprovision

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	awscreds "github.com/openshift/hive/pkg/creds/aws"

	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/openshift/installer/pkg/destroy/providers"
	"github.com/openshift/installer/pkg/types"
	awstypes "github.com/openshift/installer/pkg/types/aws"
)

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &aws.ClusterUninstaller{}
	var credsDir string
	var logLevel string
	cmd := &cobra.Command{
		Use:   "aws-tag-deprovision KEY=VALUE ...",
		Short: "Deprovision AWS assets (as created by openshift-installer) with the given tag(s)",
		Long:  "Deprovision AWS assets (as created by openshift-installer) with the given tag(s).  A resource matches the filter if any of the key/value pairs are in its tags.",
		Run: func(cmd *cobra.Command, args []string) {
			destroyer, err := createAWSDestroyer(opt, logLevel, args)
			if err != nil {
				log.WithError(err).Error("failed to create destroyer")
				return
			}

			log.Infof("Running destroyer %#v", destroyer)
			// ClusterQuota stomped in return
			if _, err := destroyer.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.Region, "region", "us-east-1", "AWS region to use")
	flags.StringVar(&credsDir, "creds-dir", "", "directory of the creds. Changes in the creds will cause the program to terminate")
	flags.StringVar(&opt.HostedZoneRole, "hosted-zone-role", "", "the role to assume when performing operations on a hosted zone owned by another account.")
	flags.StringVar(&opt.ClusterDomain, "cluster-domain", "", "the parent DNS domain of the cluster (e.g. the thing after `api.`).")
	return cmd
}

func createAWSDestroyer(o *aws.ClusterUninstaller, logLevel string, args []string) (providers.Destroyer, error) {
	metadata := &types.ClusterMetadata{
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			AWS: &awstypes.Metadata{
				Region:         o.Region,
				ClusterDomain:  o.ClusterDomain,
				HostedZoneRole: o.HostedZoneRole,
			},
		},
	}

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("incorrectly formatted filter: %q", arg)
		}
		// HACK: we need to populate the InfraID in the metadata, but we don't receive it directly.
		// Get it from the filter.
		keyParts := strings.Split(parts[0], "/")
		if len(keyParts) == 3 && keyParts[0] == "kubernetes.io" && keyParts[1] == "cluster" {
			metadata.InfraID = keyParts[2]
		}
		metadata.AWS.Identifier = append(metadata.AWS.Identifier, map[string]string{parts[0]: parts[1]})
	}

	logger, err := utils.NewLogger(logLevel)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create logger")
	}

	client, err := utils.GetClient("hiveutil-aws-tag-deprovision")
	if err != nil {
		logger.Warnf("Failed to get client: %v\n"+
			"This is expected when in standalone mode. "+
			"We expect to find your AWS credentials in one of the usual places.", err)
	}
	awscreds.ConfigureCreds(client, metadata)

	return providers.Registry[awstypes.Name](logger, metadata)
}

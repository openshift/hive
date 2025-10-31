package deprovision

import (
	"fmt"
	"os"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/hive/contrib/pkg/utils"
	openstackcreds "github.com/openshift/hive/pkg/creds/openstack"
	"github.com/openshift/installer/pkg/destroy/openstack"
	"github.com/openshift/installer/pkg/types"
	typesopenstack "github.com/openshift/installer/pkg/types/openstack"
)

// openStackOptions is the set of options to deprovision an OpenStack cluster
type openStackOptions struct {
	logLevel string
	infraID  string
	cloud    string
}

// NewDeprovisionOpenStackCommand is the entrypoint to create the OpenStack deprovision subcommand
func NewDeprovisionOpenStackCommand(logLevel string) *cobra.Command {
	opt := &openStackOptions{
		logLevel: logLevel,
	}
	cmd := &cobra.Command{
		Use:   "openstack INFRAID --cloud=OS_CLOUD",
		Short: "Deprovision OpenStack assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("failed to complete options")
			}
			if err := opt.Validate(cmd); err != nil {
				log.WithError(err).Fatal("validation failed")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.cloud, "cloud", "", "OpenStack cloud entry name from clouds.yaml for access/authentication")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *openStackOptions) Complete(cmd *cobra.Command, args []string) error {
	o.infraID = args[0]

	client, err := utils.GetClient("hiveutil-deprovision-openstack")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}
	openstackcreds.ConfigureCreds(client, nil)

	return nil
}

// Validate ensures that option values make sense
func (o *openStackOptions) Validate(cmd *cobra.Command) error {
	if o.cloud == "" {
		cmd.Usage()
		log.Info("No cloud param provided, using env var values from OS_CLOUD")
		o.cloud = os.Getenv("OS_CLOUD")
		if o.cloud == "" {
			return fmt.Errorf("no OpenStack cloud setting to use, cannot proceed")
		}
	}

	return nil
}

// Run executes the command
func (o *openStackOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	metadata := &types.ClusterMetadata{
		InfraID: o.infraID,
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			OpenStack: &typesopenstack.Metadata{
				Cloud: o.cloud,
				Identifier: map[string]string{
					"openshiftClusterID": o.infraID,
				},
			},
		},
	}

	destroyer, err := openstack.New(logger, metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

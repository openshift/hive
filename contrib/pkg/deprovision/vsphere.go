package deprovision

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openshift/installer/pkg/destroy/vsphere"
	"github.com/openshift/installer/pkg/types"
	typesvsphere "github.com/openshift/installer/pkg/types/vsphere"

	"github.com/openshift/hive/contrib/pkg/utils"
	vspherecreds "github.com/openshift/hive/pkg/creds/vsphere"
)

// vSphereOptions is the set of options to deprovision an vSphere cluster
type vSphereOptions struct {
	logLevel string
	// Only used when creds uses deprecated/legacy format (username/password with no vcenters)
	vCenters []string
	metadata *types.ClusterMetadata
}

// NewDeprovisionvSphereCommand is the entrypoint to create the vSphere deprovision subcommand
func NewDeprovisionvSphereCommand(logLevel string) *cobra.Command {
	opt := &vSphereOptions{
		logLevel: logLevel,
	}
	cmd := &cobra.Command{
		Use:   "vsphere INFRAID",
		Short: "Deprovision vSphere assets (as created by openshift-installer)",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(cmd, args); err != nil {
				log.WithError(err).Fatal("failed to complete options")
			}
			if err := opt.Run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	// Only used when creds uses deprecated/legacy format (username/password with no vcenters)
	flags.StringSliceVar(&opt.vCenters, "vsphere-vcenters", []string{}, "Domain name(s) of the vCenter(s)")
	return cmd
}

// Complete finishes parsing arguments for the command
func (o *vSphereOptions) Complete(cmd *cobra.Command, args []string) error {
	o.metadata = &types.ClusterMetadata{
		InfraID: args[0],
		ClusterPlatformMetadata: types.ClusterPlatformMetadata{
			VSphere: &typesvsphere.Metadata{
				// This entire slice will be replaced via Unmarshal() if new-style creds Secret is in play.
				// Otherwise it will be populated with copies of the single username/password from the
				// old-style creds Secret.
				VCenters: make([]typesvsphere.VCenters, len(o.vCenters)),
			},
		},
	}
	for i, vCenter := range o.vCenters {
		o.metadata.VSphere.VCenters[i] = typesvsphere.VCenters{
			VCenter: vCenter,
		}
	}

	client, err := utils.GetClient("hiveutil-deprovision-vsphere")
	if err != nil {
		return errors.Wrap(err, "failed to get client")
	}

	vspherecreds.ConfigureCreds(client, o.metadata)

	return nil
}

// Run executes the command
func (o *vSphereOptions) Run() error {
	logger, err := utils.NewLogger(o.logLevel)
	if err != nil {
		return err
	}

	destroyer, err := vsphere.New(logger, o.metadata)
	if err != nil {
		return err
	}

	// ClusterQuota stomped in return
	_, err = destroyer.Run()
	return err
}

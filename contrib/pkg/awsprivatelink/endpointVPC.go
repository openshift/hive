package awsprivatelink

import (
	"github.com/openshift/hive/contrib/pkg/awsprivatelink/endpointvpc"
	"github.com/spf13/cobra"
)

func NewEndpointVPCCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "endpointvpc",
		Short: "Manage endpoint VPCs",
		Long: `Manage endpoint VPCs. 
Setup or tear down cloud resources needed for networking between endpoint VPCs and associated VPCs.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Usage()
		},
	}
	cmd.AddCommand(endpointvpc.NewEndpointVPCAddCommand())
	cmd.AddCommand(endpointvpc.NewEndpointVPCRemoveCommand())
	return cmd
}

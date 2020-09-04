package power

import (
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// NewPowerOffCommand sets the ClusterDeployment powerState to Hibernating.
func NewPowerOffCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   `off MYCLUSTER`,
		Short: "Set ClusterDeployment powerState to Hibernating",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)

			if err := apis.AddToScheme(scheme.Scheme); err != nil {
				log.WithError(err).Fatal("error adding apis to scheme")
			}

			setClusterPowerState(args[0], hivev1.HibernatingClusterPowerState)
		},
	}

	return cmd
}

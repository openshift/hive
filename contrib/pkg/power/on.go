package power

import (
	"context"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

// NewPowerOnCommand sets the ClusterDeployment powerState to Running.
func NewPowerOnCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   `on MYCLUSTER`,
		Short: "Set ClusterDeployment powerState to Running",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)

			if err := apis.AddToScheme(scheme.Scheme); err != nil {
				log.WithError(err).Fatal("error adding apis to scheme")
			}

			setClusterPowerState(args[0], hivev1.RunningClusterPowerState)
		},
	}

	return cmd
}

func setClusterPowerState(cdName string, state hivev1.ClusterPowerState) {
	dynClient, err := utils.GetClient()
	if err != nil {
		log.WithError(err).Fatal("error creating kube clients")
	}

	cdLog := log.WithField("name", cdName)
	cd := &hivev1.ClusterDeployment{}
	err = dynClient.Get(context.Background(), types.NamespacedName{Namespace: "hive", Name: cdName}, cd)
	if err != nil {
		cdLog.WithError(err).Fatal("error looking up ClusterDeployment")
	}

	if cd.Spec.PowerState == state {
		cdLog.Infof("ClusterDeployment already in %s powerState", state)
		os.Exit(0)
	}

	cd.Spec.PowerState = state
	err = dynClient.Update(context.Background(), cd)
	if err != nil {
		cdLog.WithError(err).Fatal("error updating ClusterDeployment powerState")
	}
	cdLog.Infof("ClusterDeployment powerState set to %s", state)
}

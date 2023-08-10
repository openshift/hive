package clusterpool

import (
	"time"

	"github.com/pkg/errors"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/util/scheme"
)

type ClusterClaimOptions struct {
	Name            string
	Namespace       string
	Lifetime        time.Duration
	ClusterPoolName string

	log log.FieldLogger
}

func NewClaimClusterPoolCommand() *cobra.Command {
	opt := &ClusterClaimOptions{log: log.WithField("command", "clusterpool claim")}

	cmd := &cobra.Command{
		Use:   "claim CLUSTER_POOL_NAME CLAIM_NAME",
		Short: "claims a cluster from a ClusterPool",
		Long:  "claims a cluster from the ClusterPool in the given namespace",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			opt.ClusterPoolName = args[0]
			opt.Name = args[1]
			err := opt.run()
			if err != nil {
				opt.log.WithError(err).Fatal("Error")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opt.Namespace, "namespace", "n", "",
		"Namespace to create cluster claim in. Has to be the namespace in which the cluster pool is deployed")
	flags.DurationVar(&opt.Lifetime, "lifetime", 0, "Lifetime of the cluster claim")

	return cmd
}

func (o ClusterClaimOptions) run() error {
	scheme := scheme.GetScheme()
	claim := o.generateClaim()

	rh, err := utils.GetResourceHelper(o.log)
	if err != nil {
		return err
	}
	if len(o.Namespace) == 0 {
		o.Namespace, err = utils.DefaultNamespace()
		if err != nil {
			return errors.Wrap(err, "cannot determine default namespace")
		}
	}
	claim.Namespace = o.Namespace
	if _, err := rh.ApplyRuntimeObject(claim, scheme); err != nil {
		return err
	}

	return nil
}

func (o ClusterClaimOptions) generateClaim() *hivev1.ClusterClaim {
	cc := &hivev1.ClusterClaim{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterClaim",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: o.Name,
		},
		Spec: hivev1.ClusterClaimSpec{
			ClusterPoolName: o.ClusterPoolName,
		},
	}
	if o.Lifetime != 0 {
		cc.Spec.Lifetime = &metav1.Duration{Duration: o.Lifetime}
	}
	return cc
}

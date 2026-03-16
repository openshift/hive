package machinepool

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
)

const detachControllerName = "hiveutil-machinepool-detach"

// DetachOptions holds options for machinepool detach.
type DetachOptions struct {
	ClusterDeploymentName string
	PoolName              string
	Namespace             string
	log                   log.FieldLogger
}

// NewDetachCommand returns the machinepool detach subcommand.
func NewDetachCommand() *cobra.Command {
	opt := &DetachOptions{log: log.WithField("command", "machinepool detach")}
	cmd := &cobra.Command{
		Use:   "detach CLUSTER_DEPLOYMENT_NAME POOL_NAME",
		Short: "Detach MachineSets from MachinePool management",
		Long: `Automates the detach flow: pause ClusterDeployment reconciliation, remove Hive
labels from spoke MachineSets (hive.openshift.io/machine-pool, hive.openshift.io/managed),
delete the MachinePool on hub, then resume reconciliation. MachineSets are discovered by
label hive.openshift.io/machine-pool=<POOL_NAME> in openshift-machine-api.`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			log.SetLevel(log.InfoLevel)
			if err := opt.Complete(cmd, args); err != nil {
				opt.log.WithError(err).Fatal("Complete failed")
			}
			if err := opt.Validate(cmd); err != nil {
				opt.log.WithError(err).Fatal("Validate failed")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Run failed")
			}
		},
	}
	cmd.Flags().StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace of the ClusterDeployment (default from kubeconfig)")
	return cmd
}

// Complete fills options from args and defaults.
func (o *DetachOptions) Complete(cmd *cobra.Command, args []string) error {
	o.ClusterDeploymentName = args[0]
	o.PoolName = args[1]
	if o.Namespace == "" {
		ns, err := utils.DefaultNamespace()
		if err != nil {
			return errors.Wrap(err, "default namespace")
		}
		o.Namespace = ns
	}
	return nil
}

// Validate checks options.
func (o *DetachOptions) Validate(cmd *cobra.Command) error {
	return nil
}

// Run runs the automated detach flow: pause CD → unlabel spoke MS → delete MP → resume CD.
func (o *DetachOptions) Run() error {
	cd, err := GetClusterDeployment(o.Namespace, o.ClusterDeploymentName)
	if err != nil {
		return err
	}
	if cd.DeletionTimestamp != nil {
		return fmt.Errorf("ClusterDeployment %s/%s is being deleted", o.Namespace, o.ClusterDeploymentName)
	}
	if !cd.Spec.Installed {
		return fmt.Errorf("ClusterDeployment %s/%s is not installed yet", o.Namespace, o.ClusterDeploymentName)
	}
	if cd.Annotations != nil && cd.Annotations[constants.RelocateAnnotation] != "" {
		return fmt.Errorf("ClusterDeployment %s/%s is relocating", o.Namespace, o.ClusterDeploymentName)
	}
	if cd.Spec.ClusterMetadata == nil || cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name == "" {
		return fmt.Errorf("ClusterDeployment %s/%s has no ClusterMetadata.AdminKubeconfigSecretRef (spoke not installed or not adopted)", o.Namespace, o.ClusterDeploymentName)
	}

	hubClient, err := utils.GetClient(detachControllerName)
	if err != nil {
		return errors.Wrap(err, "get hub client")
	}

	remoteBuilder := remoteclient.NewBuilder(hubClient, cd, detachControllerName)
	remoteClient, err := remoteBuilder.Build()
	if err != nil {
		return errors.Wrap(err, "build spoke client")
	}

	o.log.Info("Pausing ClusterDeployment reconciliation...")
	if err := PauseClusterDeployment(hubClient, cd); err != nil {
		return errors.Wrap(err, "pause ClusterDeployment")
	}
	defer func() {
		o.log.Info("Resuming ClusterDeployment reconciliation...")
		cdRefreshed, _ := GetClusterDeployment(o.Namespace, o.ClusterDeploymentName)
		if cdRefreshed != nil {
			if err := ResumeClusterDeployment(hubClient, cdRefreshed); err != nil {
				o.log.WithError(err).Warn("failed to resume ClusterDeployment")
			}
		}
	}()

	o.log.Info("Removing Hive labels from spoke MachineSets...")
	n, err := UnlabelMachineSetsByPool(context.Background(), remoteClient, o.PoolName, o.log)
	if err != nil {
		return errors.Wrap(err, "unlabel MachineSets on spoke")
	}
	if n == 0 {
		o.log.Warnf("No MachineSets found with %s=%s in %s; skipping unlabel", machinePoolNameLabel, o.PoolName, machineAPINamespace)
	} else {
		o.log.Infof("Removed Hive labels from %d MachineSet(s)", n)
	}

	mpName := MachinePoolResourceName(o.ClusterDeploymentName, o.PoolName)
	mp := &hivev1.MachinePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "MachinePool",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Namespace: o.Namespace, Name: mpName},
	}
	if err := hubClient.Delete(context.Background(), mp); err != nil {
		if !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, "delete MachinePool %s/%s", o.Namespace, mpName)
		}
		o.log.Infof("MachinePool %s/%s already deleted", o.Namespace, mpName)
		return nil
	}
	o.log.Infof("MachinePool %s/%s deleted", o.Namespace, mpName)
	return nil
}

package machinepool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/resource"
)

type DetachOptions struct {
	ClusterDeploymentName string
	Namespace             string
	PoolName              string
	Force                 bool
	DryRun                bool
	log                   log.FieldLogger
	hubClient             client.Client
}

// NewDetachCommand creates the cobra command for detaching MachinePools.
func NewDetachCommand() *cobra.Command {
	opt := &DetachOptions{
		log: log.WithField("command", "machinepool detach"),
	}

	cmd := &cobra.Command{
		Use:   "detach",
		Short: "Detach MachineSets from Hive MachinePool management",
		Long: `Detach MachineSets from Hive MachinePool management.

This command follows the documented detach procedure:
1. Pauses ClusterDeployment reconciliation
2. Removes Hive management labels from MachineSets in the spoke cluster
3. Deletes the MachinePool resource from the hub cluster
4. Resumes ClusterDeployment reconciliation

After detaching, the MachineSets will continue to run but will no longer be
managed by Hive.`,
		Example: `  # Preview detach plan
  hiveutil machinepool detach -c mycluster -n mynamespace --pool worker --dry-run

  # Detach MachineSets, remove labels, and delete MachinePool
  hiveutil machinepool detach -c mycluster -n mynamespace --pool worker`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.Complete(); err != nil {
				opt.log.WithError(err).Fatal("Failed to complete options")
			}
			if err := opt.Run(); err != nil {
				opt.log.WithError(err).Fatal("Failed to detach machinepool")
			}
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opt.ClusterDeploymentName, "cluster-deployment", "c", "", "ClusterDeployment name (required)")
	flags.StringVarP(&opt.Namespace, "namespace", "n", "", "Namespace (required)")
	flags.StringVar(&opt.PoolName, "pool", "", "MachinePool name (required)")
	flags.BoolVar(&opt.DryRun, "dry-run", false, "Show what will be done without making changes (default: apply changes)")
	flags.BoolVarP(&opt.Force, "force", "f", false, "Skip interactive confirmation prompts")

	cmd.MarkFlagRequired("cluster-deployment")
	cmd.MarkFlagRequired("namespace")
	cmd.MarkFlagRequired("pool")

	return cmd
}

// Complete initializes the DetachOptions with required clients.
func (o *DetachOptions) Complete() error {
	client, err := utils.GetClient("machinepool-detach")
	if err != nil {
		return err
	}
	o.hubClient = client
	return nil
}

// Run executes the detach command.
func (o *DetachOptions) Run() error {
	ctx := context.Background()

	cd := &hivev1.ClusterDeployment{}
	key := types.NamespacedName{Namespace: o.Namespace, Name: o.ClusterDeploymentName}
	if err := o.hubClient.Get(ctx, key, cd); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.Errorf("ClusterDeployment %s/%s not found", o.Namespace, o.ClusterDeploymentName)
		}
		return errors.Wrapf(err, "failed to get ClusterDeployment %s/%s", o.Namespace, o.ClusterDeploymentName)
	}

	mpName := GetMachinePoolName(o.ClusterDeploymentName, o.PoolName)
	machinePool := &hivev1.MachinePool{}
	mpKey := types.NamespacedName{Namespace: o.Namespace, Name: mpName}
	if err := o.hubClient.Get(ctx, mpKey, machinePool); err != nil {
		if apierrors.IsNotFound(err) {
			return errors.Errorf("MachinePool %s/%s not found", o.Namespace, mpName)
		}
		return errors.Wrapf(err, "failed to get MachinePool %s/%s", o.Namespace, mpName)
	}

	if machinePool.DeletionTimestamp != nil && !o.DryRun {
		o.log.Warn("MachinePool is already being deleted. Detaching labels may prevent controller cleanup.")
	}

	remoteClient, err := remoteclient.NewBuilder(o.hubClient, cd, "machinepool-detach").Build()
	if err != nil {
		return errors.Wrap(err, "failed to create remote client to spoke cluster")
	}

	machineSets, err := o.findManagedMachineSets(ctx, remoteClient)
	if err != nil {
		return errors.Wrap(err, "failed to find managed MachineSets")
	}

	if o.DryRun {
		o.printDetachPlan(machineSets, machinePool)
		return nil
	}

	if len(machineSets) == 0 {
		o.log.Info("No MachineSets found managed by this MachinePool")
		return o.deleteMachinePool(ctx, machinePool)
	}

	if err := o.checkMachineSetsMachines(ctx, remoteClient, machineSets); err != nil {
		return errors.Wrap(err, "failed to check MachineSets for active machines")
	}

	return o.runApply(ctx, cd, remoteClient, machineSets, machinePool)
}

// runApply executes the detach procedure with reconciliation pause.
func (o *DetachOptions) runApply(ctx context.Context, cd *hivev1.ClusterDeployment, remoteClient client.Client, machineSets []machineapi.MachineSet, machinePool *hivev1.MachinePool) error {
	o.log.WithFields(log.Fields{
		"cluster": o.ClusterDeploymentName,
		"pool":    o.PoolName,
	}).Info("Detaching MachinePool")

	if err := o.withReconciliationPause(ctx, cd, func() error {
		if err := o.removeMachineSetLabels(ctx, remoteClient, machineSets); err != nil {
			return err
		}
		return o.deleteMachinePool(ctx, machinePool)
	}); err != nil {
		return err
	}

	o.log.Info("Detach completed successfully")
	return nil
}

// withReconciliationPause pauses reconciliation, executes fn, then resumes reconciliation.
func (o *DetachOptions) withReconciliationPause(ctx context.Context, cd *hivev1.ClusterDeployment, fn func() error) error {
	cdName := fmt.Sprintf("%s/%s", o.Namespace, o.ClusterDeploymentName)
	o.log.WithField("clusterdeployment", cdName).Info("Step 1/4: Pausing reconciliation")
	if err := PauseReconciliation(ctx, o.hubClient, cd); err != nil {
		return errors.Wrap(err, "failed to pause reconciliation")
	}
	o.log.Info("Reconciliation paused successfully")

	defer func() {
		var panicked interface{}
		if r := recover(); r != nil {
			panicked = r
			o.log.WithField("panic", r).Error("Panic occurred during detach, attempting to resume reconciliation")
		}
		o.log.WithField("clusterdeployment", cdName).Info("Step 4/4: Resuming reconciliation")
		if err := ResumeReconciliation(ctx, o.hubClient, cd); err != nil {
			o.log.WithError(err).Error("Failed to resume reconciliation")
			o.log.Warnf("Manually remove annotation: oc annotate clusterdeployment %s -n %s %s-",
				o.ClusterDeploymentName, o.Namespace, constants.ReconcilePauseAnnotation)
		} else {
			o.log.Info("Reconciliation resumed successfully")
		}
		if panicked != nil {
			panic(panicked)
		}
	}()

	return fn()
}

// findManagedMachineSets finds all MachineSets managed by this MachinePool.
func (o *DetachOptions) findManagedMachineSets(ctx context.Context, remoteClient client.Client) ([]machineapi.MachineSet, error) {
	machineSetList := &machineapi.MachineSetList{}
	if err := remoteClient.List(ctx, machineSetList, client.InNamespace(machineAPINamespace)); err != nil {
		return nil, errors.Wrap(err, "failed to list MachineSets")
	}

	managedMachineSets := make([]machineapi.MachineSet, 0)
	for _, ms := range machineSetList.Items {
		if poolLabel, ok := ms.Labels[machinePoolNameLabel]; ok && poolLabel == o.PoolName {
			managedMachineSets = append(managedMachineSets, ms)
		}
	}
	return managedMachineSets, nil
}

// checkMachineSetsMachines checks for active machines and prompts for confirmation if needed.
func (o *DetachOptions) checkMachineSetsMachines(ctx context.Context, remoteClient client.Client, machineSets []machineapi.MachineSet) error {
	machineSetMachines := make(map[string]int32)
	var totalMachines int32

	for _, ms := range machineSets {
		machineList := &machineapi.MachineList{}
		if err := remoteClient.List(ctx, machineList,
			client.InNamespace(machineAPINamespace),
			client.MatchingLabels(ms.Spec.Selector.MatchLabels)); err != nil {
			continue
		}
		if machineCount := int32(len(machineList.Items)); machineCount > 0 {
			machineSetMachines[ms.Name] = machineCount
			totalMachines += machineCount
		}
	}

	if totalMachines == 0 {
		return nil
	}

	o.log.WithField("total_machines", totalMachines).Warn("Active machines found")
	for msName, count := range machineSetMachines {
		o.log.WithFields(log.Fields{"machineset": msName, "machines": count}).Info("MachineSet has active machines")
	}
	o.log.Warn("Detaching will remove Hive management. Machines will continue to run but must be managed manually.")

	if o.Force {
		return nil
	}

	response, err := readConfirmationWithContext(ctx, "Continue? [y/N]: ")
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			return errors.New("detach cancelled (use --force to skip confirmation)")
		}
		return errors.Wrap(err, "failed to read user confirmation")
	}
	if response != "y" && response != "Y" && response != "yes" && response != "Yes" {
		return errors.New("detach cancelled by user (use --force to skip confirmation)")
	}
	return nil
}

// removeMachineSetLabels removes Hive management labels from MachineSets.
func (o *DetachOptions) removeMachineSetLabels(ctx context.Context, remoteClient client.Client, machineSets []machineapi.MachineSet) error {
	o.log.Info("Step 2/4: Removing labels from MachineSets")
	for i, ms := range machineSets {
		o.log.WithFields(log.Fields{
			"machineset": ms.Name,
			"progress":   fmt.Sprintf("%d/%d", i+1, len(machineSets)),
		}).Info("Removing labels from MachineSet")
		if err := UpdateMachineSetLabels(ctx, remoteClient, ms.Name, func(currentMS *machineapi.MachineSet) {
			if len(currentMS.Labels) == 0 {
				return
			}
			if poolLabel, ok := currentMS.Labels[machinePoolNameLabel]; !ok || poolLabel != o.PoolName {
				return
			}
			RemoveMachinePoolLabels(currentMS)
		}); err != nil {
			return errors.Wrapf(err, "failed to remove labels from MachineSet %s", ms.Name)
		}
	}
	o.log.WithField("count", len(machineSets)).Info("Removed labels from MachineSets")
	return nil
}

// deleteMachinePool deletes the MachinePool resource.
func (o *DetachOptions) deleteMachinePool(ctx context.Context, machinePool *hivev1.MachinePool) error {
	key := types.NamespacedName{Namespace: machinePool.Namespace, Name: machinePool.Name}
	logger := o.log.WithField("machinepool", fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name))
	logger.Info("Step 3/4: Deleting MachinePool")

	obj := &hivev1.MachinePool{}
	alreadyGone, err := resource.DeleteAnyExistingObject(o.hubClient, key, obj, logger)
	if err != nil {
		return errors.Wrapf(err, "failed to delete MachinePool")
	}
	if alreadyGone {
		logger.Info("MachinePool already deleted")
		return nil
	}

	updatedPool := &hivev1.MachinePool{}
	switch err := o.hubClient.Get(ctx, key, updatedPool); {
	case err == nil:
		if updatedPool.DeletionTimestamp != nil {
			logger.Info("MachinePool marked for deletion (has finalizer)")
		} else {
			logger.Info("MachinePool deleted successfully")
		}
	case apierrors.IsNotFound(err):
		logger.Info("MachinePool deleted successfully")
	}
	return nil
}

// printDetachPlan prints the detach plan for dry-run mode.
func (o *DetachOptions) printDetachPlan(machineSets []machineapi.MachineSet, machinePool *hivev1.MachinePool) {
	o.log.WithFields(log.Fields{
		"cluster":     o.ClusterDeploymentName,
		"pool":        o.PoolName,
		"machinepool": fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name),
		"machinesets": len(machineSets),
	}).Info("MachinePool Detach Plan")
	for _, ms := range machineSets {
		o.log.WithField("machineset", ms.Name).Info("MachineSet to detach")
	}
	o.log.WithFields(log.Fields{
		"labels": fmt.Sprintf("%s, %s", machinePoolNameLabel, constants.HiveManagedLabel),
	}).Info("Labels to be removed")
	o.log.WithField("machinepool", fmt.Sprintf("%s/%s", machinePool.Namespace, machinePool.Name)).Info("MachinePool will be deleted")
}

// Helper functions

// readConfirmationWithContext reads user confirmation from stdin with context support.
func readConfirmationWithContext(ctx context.Context, prompt string) (string, error) {
	type result struct {
		response string
		err      error
	}

	resultChan := make(chan result, 1)

	go func() {
		defer close(resultChan)
		fmt.Print(prompt)
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			resultChan <- result{err: err}
			return
		}
		response = strings.TrimSpace(response)
		resultChan <- result{response: response}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res, ok := <-resultChan:
		if !ok {
			return "", errors.New("failed to read confirmation: channel closed")
		}
		return res.response, res.err
	}
}

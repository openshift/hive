package machinepool

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/contrib/pkg/utils"
	"github.com/openshift/hive/pkg/constants"
)

// GetClusterDeployment returns the ClusterDeployment for the given namespace and name.
func GetClusterDeployment(namespace, name string) (*hivev1.ClusterDeployment, error) {
	c, err := utils.GetClient("hiveutil-machinepool")
	if err != nil {
		return nil, errors.Wrap(err, "failed to get kube client")
	}
	cd := &hivev1.ClusterDeployment{}
	err = c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, cd)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get ClusterDeployment %s/%s", namespace, name)
	}
	return cd, nil
}

// PlatformFromCD returns the platform name (aws, azure, gcp, ...) from the ClusterDeployment.
func PlatformFromCD(cd *hivev1.ClusterDeployment) (string, error) {
	if cd.Spec.Platform.AWS != nil {
		return constants.PlatformAWS, nil
	}
	if cd.Spec.Platform.Azure != nil {
		return constants.PlatformAzure, nil
	}
	if cd.Spec.Platform.GCP != nil {
		return constants.PlatformGCP, nil
	}
	if cd.Spec.Platform.IBMCloud != nil {
		return constants.PlatformIBMCloud, nil
	}
	if cd.Spec.Platform.OpenStack != nil {
		return constants.PlatformOpenStack, nil
	}
	if cd.Spec.Platform.VSphere != nil {
		return constants.PlatformVSphere, nil
	}
	if cd.Spec.Platform.Nutanix != nil {
		return constants.PlatformNutanix, nil
	}
	return "", fmt.Errorf("unsupported or unknown platform for ClusterDeployment %s/%s", cd.Namespace, cd.Name)
}

// MachinePoolResourceName returns the MachinePool resource name for a given CD and pool: cdName-poolName.
func MachinePoolResourceName(cdName, poolName string) string {
	return fmt.Sprintf("%s-%s", cdName, poolName)
}

// PauseClusterDeployment sets the reconcile-pause annotation on the ClusterDeployment so Hive skips reconciliation.
func PauseClusterDeployment(c client.Client, cd *hivev1.ClusterDeployment) error {
	if cd.Annotations == nil {
		cd.Annotations = make(map[string]string)
	}
	cd.Annotations[constants.ReconcilePauseAnnotation] = "true"
	return c.Update(context.Background(), cd)
}

// ResumeClusterDeployment removes the reconcile-pause annotation from the ClusterDeployment.
func ResumeClusterDeployment(c client.Client, cd *hivev1.ClusterDeployment) error {
	if cd.Annotations == nil {
		return nil
	}
	if _, has := cd.Annotations[constants.ReconcilePauseAnnotation]; !has {
		return nil
	}
	delete(cd.Annotations, constants.ReconcilePauseAnnotation)
	return c.Update(context.Background(), cd)
}

const (
	machinePoolNameLabel = "hive.openshift.io/machine-pool"
	hiveManagedLabel     = "hive.openshift.io/managed"
)

// updateMachineSetLabels sets and/or removes labels on ms and updates it.
// set: key -> value to set; remove: keys to delete. Pass nil to skip.
// Returns true if an update was performed.
// logger is optional; pass nil to skip logging.
func updateMachineSetLabels(ctx context.Context, c client.Client, ms *machineapi.MachineSet, set map[string]string, remove []string, logger log.FieldLogger) (bool, error) {
	if ms.Labels == nil {
		if len(set) == 0 {
			return false, nil
		}
		ms.Labels = make(map[string]string)
	}
	changed := false
	setKeys := []string{}
	for k, v := range set {
		if ms.Labels[k] != v {
			ms.Labels[k] = v
			changed = true
			setKeys = append(setKeys, fmt.Sprintf("%s=%s", k, v))
		}
	}
	removedKeys := []string{}
	for _, k := range remove {
		if _, has := ms.Labels[k]; has {
			delete(ms.Labels, k)
			changed = true
			removedKeys = append(removedKeys, k)
		}
	}
	if !changed {
		return false, nil
	}
	if err := c.Update(ctx, ms); err != nil {
		return false, err
	}
	if logger != nil {
		msg := fmt.Sprintf("Updated MachineSet %s/%s labels", ms.Namespace, ms.Name)
		if len(setKeys) > 0 {
			msg += fmt.Sprintf(" (set: %v)", setKeys)
		}
		if len(removedKeys) > 0 {
			msg += fmt.Sprintf(" (removed: %v)", removedKeys)
		}
		logger.Info(msg)
	}
	return true, nil
}

// UnlabelMachineSetsByPool lists MachineSets with hive.openshift.io/machine-pool=poolName in
// openshift-machine-api, removes hive.openshift.io/machine-pool and hive.openshift.io/managed,
// and returns the number of MachineSets updated.
// logger is optional; pass nil to skip logging.
func UnlabelMachineSetsByPool(ctx context.Context, c client.Client, poolName string, logger log.FieldLogger) (int, error) {
	list := &machineapi.MachineSetList{}
	if err := c.List(ctx, list,
		client.InNamespace(machineAPINamespace),
		client.MatchingLabels{machinePoolNameLabel: poolName},
	); err != nil {
		return 0, err
	}
	remove := []string{machinePoolNameLabel, hiveManagedLabel}
	n := 0
	for i := range list.Items {
		ms := &list.Items[i]
		changed, err := updateMachineSetLabels(ctx, c, ms, nil, remove, logger)
		if err != nil {
			return n, errors.Wrapf(err, "update MachineSet %s/%s (remove labels)", ms.Namespace, ms.Name)
		}
		if changed {
			n++
		}
	}
	return n, nil
}

// msLabelState stores original label values for rollback.
type msLabelState struct {
	name            string
	namespace       string
	hadPoolLabel    bool
	oldPoolValue    string
	hadManagedLabel bool
	oldManagedValue string
}

// LabelMachineSetsForPool adds Hive labels to MachineSets on spoke so Hive controller recognizes them.
// If labeling fails partway through, it attempts to rollback already-labeled MachineSets.
// Returns (labeled count, error). If a MachineSet is already managed by another pool, returns error without rollback.
// logger is optional; pass nil to skip logging.
func LabelMachineSetsForPool(ctx context.Context, c client.Client, machineSets []*machineapi.MachineSet, poolName string, logger log.FieldLogger) (int, error) {
	var labeledStates []msLabelState

	for _, ms := range machineSets {
		// Check if already managed by another pool — do not rollback; that would corrupt other pool's MachineSets
		if existing, ok := ms.Labels[machinePoolNameLabel]; ok && existing != poolName {
			return len(labeledStates), fmt.Errorf("MachineSet %s already managed by pool %q", ms.Name, existing)
		}

		// Save original state for rollback
		state := msLabelState{name: ms.Name, namespace: ms.Namespace}
		if ms.Labels != nil {
			state.oldPoolValue, state.hadPoolLabel = ms.Labels[machinePoolNameLabel]
			state.oldManagedValue, state.hadManagedLabel = ms.Labels[hiveManagedLabel]
		}

		fresh := &machineapi.MachineSet{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: ms.Namespace, Name: ms.Name}, fresh); err != nil {
			rollbackMachineSetLabels(ctx, c, labeledStates)
			return len(labeledStates), errors.Wrapf(err, "get MachineSet %s/%s", ms.Namespace, ms.Name)
		}
		_, err := updateMachineSetLabels(ctx, c, fresh, map[string]string{machinePoolNameLabel: poolName, hiveManagedLabel: "true"}, nil, logger)
		if err != nil {
			rollbackMachineSetLabels(ctx, c, labeledStates)
			return len(labeledStates), errors.Wrapf(err, "update MachineSet %s/%s labels", ms.Namespace, ms.Name)
		}
		labeledStates = append(labeledStates, state)
	}
	return len(labeledStates), nil
}

// rollbackMachineSetLabels restores original label values on MachineSets.
func rollbackMachineSetLabels(ctx context.Context, c client.Client, states []msLabelState) {
	for _, state := range states {
		fresh := &machineapi.MachineSet{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: state.namespace, Name: state.name}, fresh); err != nil {
			continue
		}
		set := make(map[string]string)
		var remove []string
		if state.hadPoolLabel {
			set[machinePoolNameLabel] = state.oldPoolValue
		} else {
			remove = append(remove, machinePoolNameLabel)
		}
		if state.hadManagedLabel {
			set[hiveManagedLabel] = state.oldManagedValue
		} else {
			remove = append(remove, hiveManagedLabel)
		}
		updateMachineSetLabels(ctx, c, fresh, set, remove, nil)
	}
}

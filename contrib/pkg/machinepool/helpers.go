package machinepool

import (
	"context"
	"fmt"
	"maps"
	"os"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	machineAPINamespace  = "openshift-machine-api"
	machinePoolNameLabel = "hive.openshift.io/machine-pool"
)

// GetPlatformFromClusterDeployment extracts the platform type from a ClusterDeployment.
func GetPlatformFromClusterDeployment(cd *hivev1.ClusterDeployment) string {
	switch {
	case cd.Spec.Platform.AWS != nil:
		return constants.PlatformAWS
	case cd.Spec.Platform.GCP != nil:
		return constants.PlatformGCP
	case cd.Spec.Platform.Azure != nil:
		return constants.PlatformAzure
	case cd.Spec.Platform.VSphere != nil:
		return constants.PlatformVSphere
	case cd.Spec.Platform.Nutanix != nil:
		return constants.PlatformNutanix
	case cd.Spec.Platform.IBMCloud != nil:
		return constants.PlatformIBMCloud
	case cd.Spec.Platform.OpenStack != nil:
		return constants.PlatformOpenStack
	}
	return constants.PlatformUnknown
}

// GetMachinePoolName generates the MachinePool name from cluster and pool names.
func GetMachinePoolName(clusterName, poolName string) string {
	return fmt.Sprintf("%s-%s", clusterName, poolName)
}

// MachineSetUpdater is a function type for updating MachineSet labels.
type MachineSetUpdater func(*machineapi.MachineSet)

// UpdateMachineSetLabels updates MachineSet labels using retry logic.
func UpdateMachineSetLabels(ctx context.Context, remoteClient client.Client, msName string, updater MachineSetUpdater) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		currentMS := &machineapi.MachineSet{}
		key := client.ObjectKey{Namespace: machineAPINamespace, Name: msName}
		if err := remoteClient.Get(ctx, key, currentMS); err != nil {
			return err
		}
		updater(currentMS)
		return remoteClient.Update(ctx, currentMS)
	})
}

// AddMachinePoolLabels adds Hive management labels to a MachineSet.
func AddMachinePoolLabels(ms *machineapi.MachineSet, poolName string) {
	if ms.Labels == nil {
		ms.Labels = make(map[string]string)
	}
	ms.Labels[machinePoolNameLabel] = poolName
	ms.Labels[constants.HiveManagedLabel] = "true"
}

// RemoveMachinePoolLabels removes Hive management labels from a MachineSet.
func RemoveMachinePoolLabels(ms *machineapi.MachineSet) {
	if len(ms.Labels) == 0 {
		return
	}
	delete(ms.Labels, machinePoolNameLabel)
	delete(ms.Labels, constants.HiveManagedLabel)
}

// RestoreMachineSetLabels restores original labels to a MachineSet.
func RestoreMachineSetLabels(ms *machineapi.MachineSet, originalLabels map[string]string) {
	if ms.Labels == nil {
		if len(originalLabels) > 0 {
			ms.Labels = make(map[string]string, len(originalLabels))
			maps.Copy(ms.Labels, originalLabels)
		}
		return
	}
	// Remove Hive management labels
	delete(ms.Labels, machinePoolNameLabel)
	delete(ms.Labels, constants.HiveManagedLabel)
	// Restore original values if they existed
	if originalLabels != nil {
		for _, key := range []string{machinePoolNameLabel, constants.HiveManagedLabel} {
			if val, ok := originalLabels[key]; ok {
				ms.Labels[key] = val
			}
		}
	}
}

// PauseReconciliation pauses ClusterDeployment reconciliation.
func PauseReconciliation(ctx context.Context, hubClient client.Client, cd *hivev1.ClusterDeployment) error {
	return updateReconciliationAnnotation(ctx, hubClient, cd, true)
}

// ResumeReconciliation resumes ClusterDeployment reconciliation.
func ResumeReconciliation(ctx context.Context, hubClient client.Client, cd *hivev1.ClusterDeployment) error {
	return updateReconciliationAnnotation(ctx, hubClient, cd, false)
}

// updateReconciliationAnnotation updates the reconciliation pause annotation.
func updateReconciliationAnnotation(ctx context.Context, hubClient client.Client, cd *hivev1.ClusterDeployment, pause bool) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		currentCD := &hivev1.ClusterDeployment{}
		key := types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}
		if err := hubClient.Get(ctx, key, currentCD); err != nil {
			return err
		}
		if isReconciliationPaused(currentCD) == pause {
			return nil
		}
		if currentCD.Annotations == nil {
			currentCD.Annotations = make(map[string]string)
		}
		if pause {
			currentCD.Annotations[constants.ReconcilePauseAnnotation] = "true"
		} else {
			delete(currentCD.Annotations, constants.ReconcilePauseAnnotation)
			if len(currentCD.Annotations) == 0 {
				currentCD.Annotations = nil
			}
		}
		return hubClient.Update(ctx, currentCD)
	})
}

// isReconciliationPaused checks if reconciliation is paused for a ClusterDeployment.
func isReconciliationPaused(cd *hivev1.ClusterDeployment) bool {
	if cd.Annotations == nil {
		return false
	}
	paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation])
	return err == nil && paused
}

// CreatePrinter creates a ResourcePrinter based on the output format
func CreatePrinter(outputFormat string) printers.ResourcePrinter {
	if outputFormat == "yaml" || outputFormat == "" {
		return &printers.YAMLPrinter{}
	}
	return &printers.JSONPrinter{}
}

// PrintObjects prints a list of runtime objects using the specified printer
func PrintObjects(objects []runtime.Object, scheme *runtime.Scheme, printer printers.ResourcePrinter) {
	typeSetterPrinter := printers.NewTypeSetter(scheme).ToPrinter(printer)
	switch len(objects) {
	case 0:
		return
	case 1:
		typeSetterPrinter.PrintObj(objects[0], os.Stdout)
	default:
		list := &metav1.List{
			TypeMeta: metav1.TypeMeta{
				Kind:       "List",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ListMeta: metav1.ListMeta{},
		}
		meta.SetList(list, objects)
		typeSetterPrinter.PrintObj(list, os.Stdout)
	}
}

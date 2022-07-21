package utils

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// UpdateConditionCheck tests whether a condition should be updated from the
// old condition to the new condition. Returns true if the condition should
// be updated.
type UpdateConditionCheck func(oldReason, oldMessage, newReason, newMessage string) bool

// UpdateConditionAlways returns true. The condition will always be updated.
func UpdateConditionAlways(_, _, _, _ string) bool {
	return true
}

// UpdateConditionNever return false. The condition will never be updated,
// unless there is a change in the status of the condition.
func UpdateConditionNever(_, _, _, _ string) bool {
	return false
}

// UpdateConditionIfReasonOrMessageChange returns true if there is a change
// in the reason or the message of the condition.
func UpdateConditionIfReasonOrMessageChange(oldReason, oldMessage, newReason, newMessage string) bool {
	return oldReason != newReason ||
		oldMessage != newMessage
}

func shouldUpdateCondition(
	oldStatus corev1.ConditionStatus, oldReason, oldMessage string,
	newStatus corev1.ConditionStatus, newReason, newMessage string,
	updateConditionCheck UpdateConditionCheck,
) bool {
	if oldStatus != newStatus {
		return true
	}
	return updateConditionCheck(oldReason, oldMessage, newReason, newMessage)
}

// InitializeClusterDeploymentConditions initializes the given set of conditions for the first time, set with Status Unknown.
// If the conditions already exist, they are not affected.
// The first return is the updated list of conditions (those passed in via `existingConditions` plus any new ones.)
// The second return indicates whether we made any changes. It should be used by the caller to decide whether to perform a
// Status().Update().
func InitializeClusterDeploymentConditions(existingConditions []hivev1.ClusterDeploymentCondition, conditionsToBeAdded []hivev1.ClusterDeploymentConditionType) ([]hivev1.ClusterDeploymentCondition, bool) {
	now := metav1.Now()
	changed := false
	for _, conditionType := range conditionsToBeAdded {
		if FindCondition(existingConditions, conditionType) == nil {
			existingConditions = append(
				existingConditions,
				hivev1.ClusterDeploymentCondition{
					Type:               conditionType,
					Status:             corev1.ConditionUnknown,
					Reason:             hivev1.InitializedConditionReason,
					Message:            "Condition Initialized",
					LastTransitionTime: now,
					LastProbeTime:      now,
				})
			changed = true
		}
	}
	return existingConditions, changed
}

// SetClusterDeploymentCondition sets a condition on a ClusterDeployment resource's status
func SetClusterDeploymentCondition(
	conditions []hivev1.ClusterDeploymentCondition,
	conditionType hivev1.ClusterDeploymentConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.ClusterDeploymentCondition {
	newConditions, _ := SetClusterDeploymentConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetClusterDeploymentConditionWithChangeCheck sets a condition on a ClusterDeployment resource's status.
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetClusterDeploymentConditionWithChangeCheck(
	conditions []hivev1.ClusterDeploymentCondition,
	conditionType hivev1.ClusterDeploymentConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.ClusterDeploymentCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.ClusterDeploymentCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
		changed = true
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	if changed {
		conditions = SortClusterDeploymentConditions(conditions)
	}
	return conditions, changed
}

// SortClusterDeploymentConditions sorts the cluster deployment conditions such that conditions in their undesired/error
// state are at the top, followed by conditions in their desired/expected state, with conditions that have unknown status
// at the last. All conditions that are undesired, desired or unknown are sorted in alphabetical order of their type
func SortClusterDeploymentConditions(conditions []hivev1.ClusterDeploymentCondition) []hivev1.ClusterDeploymentCondition {
	sort.SliceStable(conditions, func(i, j int) bool {
		if conditions[i].Status != corev1.ConditionUnknown && conditions[j].Status == corev1.ConditionUnknown {
			return true
		}
		if conditions[i].Status == corev1.ConditionUnknown && conditions[j].Status != corev1.ConditionUnknown {
			return false
		}
		if !IsConditionInDesiredState(conditions[i]) && IsConditionInDesiredState(conditions[j]) {
			return true
		}
		if IsConditionInDesiredState(conditions[i]) && !IsConditionInDesiredState(conditions[j]) {
			return false
		}
		return conditions[i].Type < conditions[j].Type
	})
	return conditions
}

// InitializeClusterClaimConditions initializes the given set of conditions for the first time, set with Status Unknown.
// If the conditions already exist, they are not affected.
// The first return is the updated list of conditions (those passed in via `existingConditions` plus any new ones.)
// The second return indicates whether we made any changes. It should be used by the caller to decide whether to perform a
// Status().Update().
func InitializeClusterClaimConditions(existingConditions []hivev1.ClusterClaimCondition,
	conditionsToBeAdded []hivev1.ClusterClaimConditionType) ([]hivev1.ClusterClaimCondition, bool) {
	now := metav1.Now()
	changed := false
	for _, conditionType := range conditionsToBeAdded {
		if FindCondition(existingConditions, conditionType) == nil {
			existingConditions = append(
				existingConditions,
				hivev1.ClusterClaimCondition{
					Type:               conditionType,
					Status:             corev1.ConditionUnknown,
					Reason:             hivev1.InitializedConditionReason,
					Message:            "Condition Initialized",
					LastTransitionTime: now,
					LastProbeTime:      now,
				})
			changed = true
		}
	}
	return existingConditions, changed
}

// SetClusterClaimCondition sets a condition on a ClusterClaim resource's status
func SetClusterClaimCondition(
	conditions []hivev1.ClusterClaimCondition,
	conditionType hivev1.ClusterClaimConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.ClusterClaimCondition {
	newConditions, _ := SetClusterClaimConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetClusterClaimConditionWithChangeCheck sets a condition on a ClusterClaim resource's status.
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetClusterClaimConditionWithChangeCheck(
	conditions []hivev1.ClusterClaimCondition,
	conditionType hivev1.ClusterClaimConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.ClusterClaimCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.ClusterClaimCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
		changed = true
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

// InitializeClusterPoolConditions initializes the given set of conditions for the first time, set with Status Unknown.
// If the conditions already exist, they are not affected.
// The first return is the updated list of conditions (those passed in via `existingConditions` plus any new ones.)
// The second return indicates whether we made any changes. It should be used by the caller to decide whether to perform a
// Status().Update().
func InitializeClusterPoolConditions(existingConditions []hivev1.ClusterPoolCondition,
	conditionsToBeAdded []hivev1.ClusterPoolConditionType) ([]hivev1.ClusterPoolCondition, bool) {
	now := metav1.Now()
	changed := false
	for _, conditionType := range conditionsToBeAdded {
		if FindCondition(existingConditions, conditionType) == nil {
			existingConditions = append(
				existingConditions,
				hivev1.ClusterPoolCondition{
					Type:               conditionType,
					Status:             corev1.ConditionUnknown,
					Reason:             hivev1.InitializedConditionReason,
					Message:            "Condition Initialized",
					LastTransitionTime: now,
					LastProbeTime:      now,
				})
			changed = true
		}
	}
	return existingConditions, changed
}

// SetClusterPoolCondition sets a condition on a ClusterPool resource's status
func SetClusterPoolCondition(
	conditions []hivev1.ClusterPoolCondition,
	conditionType hivev1.ClusterPoolConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.ClusterPoolCondition {
	newConditions, _ := SetClusterPoolConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetClusterPoolConditionWithChangeCheck sets a condition on a ClusterPool resource's status.
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetClusterPoolConditionWithChangeCheck(
	conditions []hivev1.ClusterPoolCondition,
	conditionType hivev1.ClusterPoolConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.ClusterPoolCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.ClusterPoolCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
		changed = true
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

// SetClusterProvisionCondition sets a condition on a ClusterProvision resource's status
func SetClusterProvisionCondition(
	conditions []hivev1.ClusterProvisionCondition,
	conditionType hivev1.ClusterProvisionConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.ClusterProvisionCondition {
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				hivev1.ClusterProvisionCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastProbeTime:      now,
				},
			)
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
		}
	}
	return conditions
}

// SetSyncCondition sets a condition on a SyncSet or resource's status
func SetSyncCondition(
	conditions []hivev1.SyncCondition,
	conditionType hivev1.SyncConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.SyncCondition {
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				hivev1.SyncCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastProbeTime:      now,
				},
			)
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
		}
	}
	return conditions
}

// SetDNSZoneCondition sets a condition on a DNSZone resource's status
func SetDNSZoneCondition(
	conditions []hivev1.DNSZoneCondition,
	conditionType hivev1.DNSZoneConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.DNSZoneCondition {
	newConditions, _ := SetDNSZoneConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetDNSZoneConditionWithChangeCheck sets a condition on a DNSZone resource's status
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetDNSZoneConditionWithChangeCheck(
	conditions []hivev1.DNSZoneCondition,
	conditionType hivev1.DNSZoneConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.DNSZoneCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				hivev1.DNSZoneCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastProbeTime:      now,
				},
			)
			changed = true
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

// InitializeMachinePoolConditions initializes the given set of conditions for the first time, set with Status Unknown
func InitializeMachinePoolConditions(existingConditions []hivev1.MachinePoolCondition, conditionsToBeAdded []hivev1.MachinePoolConditionType) ([]hivev1.MachinePoolCondition, bool) {
	now := metav1.Now()
	changed := false
	for _, conditionType := range conditionsToBeAdded {
		if FindCondition(existingConditions, conditionType) == nil {
			existingConditions = append(
				existingConditions,
				hivev1.MachinePoolCondition{
					Type:               conditionType,
					Status:             corev1.ConditionUnknown,
					Reason:             hivev1.InitializedConditionReason,
					Message:            "Condition Initialized",
					LastTransitionTime: now,
					LastProbeTime:      now,
				})
			changed = true
		}
	}
	return existingConditions, changed
}

// SetMachinePoolCondition sets a condition on a MachinePool resource's status
func SetMachinePoolCondition(
	conditions []hivev1.MachinePoolCondition,
	conditionType hivev1.MachinePoolConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.MachinePoolCondition {
	newConditions, _ := SetMachinePoolConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetMachinePoolConditionWithChangeCheck sets a condition on a MachinePool resource's status.
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetMachinePoolConditionWithChangeCheck(
	conditions []hivev1.MachinePoolCondition,
	conditionType hivev1.MachinePoolConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.MachinePoolCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.MachinePoolCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
		changed = true
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

// SetClusterDeprovisionCondition sets a condition on a ClusterDeprovision resource's status
func SetClusterDeprovisionCondition(
	conditions []hivev1.ClusterDeprovisionCondition,
	conditionType hivev1.ClusterDeprovisionConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) []hivev1.ClusterDeprovisionCondition {
	newConditions, _ := SetClusterDeprovisionConditionWithChangeCheck(
		conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	return newConditions
}

// SetClusterDeprovisionConditionWithChangeCheck sets a condition on a ClusterDeprovision resource's status
// It returns the conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetClusterDeprovisionConditionWithChangeCheck(
	conditions []hivev1.ClusterDeprovisionCondition,
	conditionType hivev1.ClusterDeprovisionConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.ClusterDeprovisionCondition, bool) {
	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
			conditions = append(
				conditions,
				hivev1.ClusterDeprovisionCondition{
					Type:               conditionType,
					Status:             status,
					Reason:             reason,
					Message:            message,
					LastTransitionTime: now,
					LastProbeTime:      now,
				},
			)
			changed = true
		}
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

// SetClusterInstallConditionWithChangeCheck sets a condition in the list of status conditions
// for a ClusterInstall implementation.
// It returns the resulting conditions as well a boolean indicating whether there was a change made
// to the conditions.
func SetClusterInstallConditionWithChangeCheck(
	conditions []hivev1.ClusterInstallCondition,
	conditionType hivev1.ClusterInstallConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck UpdateConditionCheck,
) ([]hivev1.ClusterInstallCondition, bool) {

	changed := false
	now := metav1.Now()
	existingCondition := FindCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.ClusterInstallCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
		changed = true
	} else {
		if shouldUpdateCondition(
			existingCondition.Status, existingCondition.Reason, existingCondition.Message,
			status, reason, message,
			updateConditionCheck,
		) {
			if existingCondition.Status != status {
				existingCondition.LastTransitionTime = now
			}
			existingCondition.Status = status
			existingCondition.Reason = reason
			existingCondition.Message = message
			existingCondition.LastProbeTime = now
			changed = true
		}
	}
	return conditions, changed
}

func FindCondition[C hivev1.Condition, T hivev1.ConditionType](conditions []C, conditionType T) *C {
	for i, condition := range conditions {
		if condition.ConditionType().String() == conditionType.String() {
			return &conditions[i]
		}
	}
	return nil
}

// IsConditionWithPositivePolarity checks if cluster deployment condition has positive polarity
func IsConditionWithPositivePolarity(conditionType hivev1.ClusterDeploymentConditionType) bool {
	for _, condition := range hivev1.PositivePolarityClusterDeploymentConditions {
		if condition == conditionType {
			return true
		}
	}
	return false
}

// IsConditionInDesiredState checks if the condition status is in its desired/expected state
func IsConditionInDesiredState(condition hivev1.ClusterDeploymentCondition) bool {
	return (IsConditionWithPositivePolarity(condition.Type) && condition.Status == corev1.ConditionTrue) ||
		(!IsConditionWithPositivePolarity(condition.Type) && condition.Status == corev1.ConditionFalse)
}

// AreAllConditionsInDesiredState checks if all cluster deployment conditions are in their desired state
func AreAllConditionsInDesiredState(conditions []hivev1.ClusterDeploymentCondition) bool {
	// cluster deployment conditions are sorted to have error conditions at the top
	if conditions[0].Status == corev1.ConditionUnknown || IsConditionInDesiredState(conditions[0]) {
		return true
	}
	return false
}

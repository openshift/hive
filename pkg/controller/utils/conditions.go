package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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
	existingCondition := FindClusterDeploymentCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
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
	existingCondition := FindClusterProvisionCondition(conditions, conditionType)
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
	existingCondition := FindSyncCondition(conditions, conditionType)
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
	existingCondition := FindDNSZoneCondition(conditions, conditionType)
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
	existingCondition := FindMachinePoolCondition(conditions, conditionType)
	if existingCondition == nil {
		if status == corev1.ConditionTrue {
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

// FindClusterDeploymentCondition finds in the condition that has the
// specified condition type in the given list. If none exists, then returns nil.
func FindClusterDeploymentCondition(conditions []hivev1.ClusterDeploymentCondition, conditionType hivev1.ClusterDeploymentConditionType) *hivev1.ClusterDeploymentCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// FindClusterProvisionCondition finds in the condition that has the
// specified condition type in the given list. If none exists, then returns nil.
func FindClusterProvisionCondition(conditions []hivev1.ClusterProvisionCondition, conditionType hivev1.ClusterProvisionConditionType) *hivev1.ClusterProvisionCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// FindSyncCondition finds in the condition that has the specified condition type
// in the given list. If none exists, then returns nil.
func FindSyncCondition(conditions []hivev1.SyncCondition, conditionType hivev1.SyncConditionType) *hivev1.SyncCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// FindDNSZoneCondition finds in the condition that has the
// specified condition type in the given list. If none exists, then returns nil.
func FindDNSZoneCondition(conditions []hivev1.DNSZoneCondition, conditionType hivev1.DNSZoneConditionType) *hivev1.DNSZoneCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// FindMachinePoolCondition finds in the condition that has the
// specified condition type in the given list. If none exists, then returns nil.
func FindMachinePoolCondition(conditions []hivev1.MachinePoolCondition, conditionType hivev1.MachinePoolConditionType) *hivev1.MachinePoolCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

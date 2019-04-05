/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
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

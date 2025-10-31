package hive

import (
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SetHiveConfigCondition(
	conditions []hivev1.HiveConfigCondition,
	conditionType hivev1.HiveConfigConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
) []hivev1.HiveConfigCondition {
	now := metav1.Now()
	existingCondition := FindHiveConfigCondition(conditions, conditionType)
	if existingCondition == nil {
		conditions = append(
			conditions,
			hivev1.HiveConfigCondition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Message:            message,
				LastTransitionTime: now,
				LastProbeTime:      now,
			},
		)
	} else {
		if existingCondition.Status != status ||
			existingCondition.Message != message ||
			existingCondition.Reason != reason {
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

func FindHiveConfigCondition(conditions []hivev1.HiveConfigCondition, conditionType hivev1.HiveConfigConditionType) *hivev1.HiveConfigCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// InitializeHiveConfigConditions initializes the given set of conditions for the first time, set with Status Unknown
func InitializeHiveConfigConditions(existingConditions []hivev1.HiveConfigCondition, conditionsToBeAdded []hivev1.HiveConfigConditionType) ([]hivev1.HiveConfigCondition, bool) {
	now := metav1.Now()
	changed := false
	for _, conditionType := range conditionsToBeAdded {
		if FindHiveConfigCondition(existingConditions, conditionType) == nil {
			existingConditions = append(
				existingConditions,
				hivev1.HiveConfigCondition{
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

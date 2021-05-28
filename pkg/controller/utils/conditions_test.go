package utils

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func TestSortedClusterDeploymentConditions(t *testing.T) {
	addConditions := map[hivev1.ClusterDeploymentConditionType]corev1.ConditionStatus{
		hivev1.ClusterImageSetNotFoundCondition:               corev1.ConditionFalse,
		hivev1.DNSNotReadyCondition:                           corev1.ConditionTrue,
		hivev1.SyncSetFailedCondition:                         corev1.ConditionUnknown,
		hivev1.ProvisionStoppedCondition:                      corev1.ConditionUnknown,
		hivev1.AWSPrivateLinkReadyClusterDeploymentCondition:  corev1.ConditionTrue,
		hivev1.AWSPrivateLinkFailedClusterDeploymentCondition: corev1.ConditionTrue,
	}
	var conditions []hivev1.ClusterDeploymentCondition
	for condType, condStatus := range addConditions {
		conditions = SetClusterDeploymentCondition(conditions, condType, condStatus, "test-reason",
			"test message", UpdateConditionAlways)
	}
	if assert.NotNil(t, conditions, "conditions were not set") {
		if assert.Len(t, conditions, len(addConditions), "unexpected conditions found") {
			compareConditions := []hivev1.ClusterDeploymentConditionType{
				// undesired
				hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
				hivev1.DNSNotReadyCondition,
				// desired
				hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
				hivev1.ClusterImageSetNotFoundCondition,
				// unknown
				hivev1.ProvisionStoppedCondition,
				hivev1.SyncSetFailedCondition,
			}
			for i, cond := range conditions {
				assert.Equal(t, compareConditions[i], cond.Type, "conditions not sorted as expected")
			}
		}
	}
}

package assert

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	testifyassert "github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

// BetweenTimes asserts that the time is within the time window, inclusive of the start and end times.
//
//	assert.BetweenTimes(t, time.Now(), time.Now().Add(-10*time.Second), time.Now().Add(10*time.Second))
func BetweenTimes(t *testing.T, actual, startTime, endTime time.Time, msgAndArgs ...interface{}) bool {
	if actual.Before(startTime) {
		return testifyassert.Fail(t, fmt.Sprintf("Actual time %v is before start time %v", actual, startTime), msgAndArgs...)
	}
	if actual.After(endTime) {
		return testifyassert.Fail(t, fmt.Sprintf("Actual time %v is after end time %v", actual, endTime), msgAndArgs...)
	}
	return true
}

func AssertAllContainersHaveEnvVar(t *testing.T, podSpec *corev1.PodSpec, key, value string) {
	for _, c := range podSpec.Containers {
		found := false
		foundCtr := 0
		for _, ev := range c.Env {
			if ev.Name == key {
				foundCtr++
				found = ev.Value == value
			}
		}
		testifyassert.True(t, found, "env var %s=%s not found on container %s", key, value, c.Name)
		testifyassert.Equal(t, 1, foundCtr, "found %d occurrences of env var %s on container %s", foundCtr, key, c.Name)
	}
}

// findClusterDeploymentCondition finds the specified condition type in the given list of cluster deployment conditions.
// If none exists, then returns nil.
func findClusterDeploymentCondition(conditions []hivev1.ClusterDeploymentCondition, conditionType hivev1.ClusterDeploymentConditionType) *hivev1.ClusterDeploymentCondition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func findCDCCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i, condition := range conditions {
		if condition.Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// AssertConditionStatus asserts if a condition is present on the cluster deployment and has the expected status
func AssertConditionStatus(t *testing.T, cd *hivev1.ClusterDeployment, condType hivev1.ClusterDeploymentConditionType, status corev1.ConditionStatus) {
	condition := findClusterDeploymentCondition(cd.Status.Conditions, condType)
	if testifyassert.NotNilf(t, condition, "did not find expected condition type: %v", condType) {
		testifyassert.Equal(t, string(status), string(condition.Status), "condition found with unexpected status")
	}
}

// AssertConditions asserts if the expected conditions are present on the cluster deployment.
// It also asserts if those conditions have the expected status, reason, and (optionally) message.
func AssertConditions(t *testing.T, cd *hivev1.ClusterDeployment, expectedConditions []hivev1.ClusterDeploymentCondition) {
	testifyassert.LessOrEqual(t, len(expectedConditions), len(cd.Status.Conditions), "some conditions are not present")
	for _, expectedCond := range expectedConditions {
		condition := findClusterDeploymentCondition(cd.Status.Conditions, expectedCond.Type)
		if testifyassert.NotNilf(t, condition, "did not find expected condition type: %v", expectedCond.Type) {
			testifyassert.Equal(t, expectedCond.Status, condition.Status, "condition found with unexpected status")
			testifyassert.Equal(t, expectedCond.Reason, condition.Reason, "condition found with unexpected reason")
			// Optionally validate the message
			if expectedCond.Message != "" {
				testifyassert.Equal(t, expectedCond.Message, condition.Message, "condition found with unexpected message")
			}
		}
	}
}

// AssertConditions asserts if the expected conditions are present on the cluster deployment.
// It also asserts if those conditions have the expected status, reason, and (optionally) message.
func AssertCDCConditions(t *testing.T, cdc *hivev1.ClusterDeploymentCustomization, expectedConditions []metav1.Condition) {
	testifyassert.LessOrEqual(t, len(expectedConditions), len(cdc.Status.Conditions), "some conditions are not present")
	for _, expectedCond := range expectedConditions {
		condition := findCDCCondition(cdc.Status.Conditions, expectedCond.Type)
		if testifyassert.NotNilf(t, condition, "did not find expected condition type: %v", expectedCond.Type) {
			testifyassert.Equal(t, expectedCond.Status, condition.Status, "condition found with unexpected status")
			testifyassert.Equal(t, expectedCond.Reason, condition.Reason, "condition found with unexpected reason")
			// Optionally validate the message
			if expectedCond.Message != "" {
				testifyassert.Equal(t, expectedCond.Message, condition.Message, "condition found with unexpected message")
			}
		}
	}
}

// AssertEqualWhereItCounts compares two runtime.Objects, ignoring their ResourceVersion and TypeMeta, asserting that they
// are otherwise equal.
// This and cleanRVAndTypeMeta were borrowed/adapted from:
// https://github.com/openshift/ci-tools/blob/179a0edb6c003aa95ae1332692c5b4c79e58f674/pkg/testhelper/testhelper.go#L157-L175
func AssertEqualWhereItCounts(t *testing.T, x, y runtime.Object, msg string) {
	xCopy := x.DeepCopyObject()
	yCopy := y.DeepCopyObject()
	cleanRVAndTypeMeta(xCopy)
	cleanRVAndTypeMeta(yCopy)
	diff := cmp.Diff(xCopy, yCopy)
	testifyassert.Empty(t, diff, msg)
}

func cleanRVAndTypeMeta(r runtime.Object) {
	if metaObject, ok := r.(metav1.Object); ok {
		metaObject.SetResourceVersion("")
	}
	if typeObject, ok := r.(interface{ SetGroupVersionKind(schema.GroupVersionKind) }); ok {
		typeObject.SetGroupVersionKind(schema.GroupVersionKind{})
	}
}

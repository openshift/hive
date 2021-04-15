package assert

import (
	"fmt"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

// BetweenTimes asserts that the time is within the time window, inclusive of the start and end times.
//
//   assert.BetweenTimes(t, time.Now(), time.Now().Add(-10*time.Second), time.Now().Add(10*time.Second))
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

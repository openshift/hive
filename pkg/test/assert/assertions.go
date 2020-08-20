package assert

import (
	"fmt"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
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

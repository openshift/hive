package utils

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// fakeReconciler is a helper that returns preconfigured results and errors.
type fakeReconciler struct {
	result reconcile.Result
	err    error
}

func (f *fakeReconciler) Reconcile(_ context.Context, _ reconcile.Request) (reconcile.Result, error) {
	return f.result, f.err
}

func newTestDelayingReconciler(inner reconcile.Reconciler) *delayingReconciler {
	return NewDelayingReconciler(inner, log.WithField("test", true)).(*delayingReconciler)
}

var testRequest = reconcile.Request{
	NamespacedName: types.NamespacedName{Namespace: "test-ns", Name: "test-name"},
}

// --- isForbidden tests (backward compat) ---

func TestIsForbidden_401(t *testing.T) {
	assert.True(t, isForbidden(fmt.Errorf("error code 401 unauthorized")))
}

func TestIsForbidden_403(t *testing.T) {
	assert.True(t, isForbidden(fmt.Errorf("error code 403 forbidden")))
}

func TestIsForbidden_Nil(t *testing.T) {
	assert.False(t, isForbidden(nil))
}

func TestIsForbidden_OtherError(t *testing.T) {
	assert.False(t, isForbidden(fmt.Errorf("connection timeout")))
}

func TestDelayingReconciler_ForbiddenTriggersFixedDelay(t *testing.T) {
	inner := &fakeReconciler{err: fmt.Errorf("status code 403 forbidden")}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, requeueDelay, result.RequeueAfter)
}

// --- Non-wrapped errors pass through unchanged ---

func TestDelayingReconciler_NonWrappedErrorPassesThrough(t *testing.T) {
	origErr := fmt.Errorf("some random error")
	inner := &fakeReconciler{err: origErr}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, origErr, err)
	assert.Equal(t, reconcile.Result{}, result)
}

func TestDelayingReconciler_SuccessPassesThrough(t *testing.T) {
	inner := &fakeReconciler{result: reconcile.Result{Requeue: true}}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.True(t, result.Requeue)
}

// --- ErrorWithCustomBackoff type assertion ---

// testError is a sentinel error type for testing Match functions.
type testThrottleError struct{ msg string }

func (e *testThrottleError) Error() string { return e.msg }

type testOtherError struct{ msg string }

func (e *testOtherError) Error() string { return e.msg }

func TestErrorWithCustomBackoff_Unwrap(t *testing.T) {
	inner := &testThrottleError{msg: "throttled"}
	ecb := ErrorWithCustomBackoff{
		error:          inner,
		CustomBackoffs: nil,
	}
	assert.True(t, errors.Is(ecb, inner))
	var te *testThrottleError
	assert.True(t, errors.As(ecb, &te))
	assert.Equal(t, "throttled", te.msg)
}

func TestErrorWithCustomBackoff_Error(t *testing.T) {
	ecb := ErrorWithCustomBackoff{
		error: fmt.Errorf("boom"),
	}
	assert.Equal(t, "boom", ecb.Error())
}

func TestErrorWithCustomBackoff_NilErr(t *testing.T) {
	ecb := ErrorWithCustomBackoff{}
	assert.Equal(t, "", ecb.Error())
}

func TestNewErrorWithCustomBackoff(t *testing.T) {
	cb := &CustomBackoff{Name: "test", MinDelay: time.Second, MaxDelay: time.Minute}
	ecb := NewErrorWithCustomBackoff(fmt.Errorf("boom"), []*CustomBackoff{cb})
	assert.Equal(t, "boom", ecb.Error())
	assert.Len(t, ecb.CustomBackoffs, 1)
	assert.Equal(t, "test", ecb.CustomBackoffs[0].Name)
}

// --- Exponential backoff with CustomBackoff ---

func newTestCustomBackoff(minDelay, maxDelay time.Duration, matchFunc func(error) bool) *CustomBackoff {
	return &CustomBackoff{
		Name:     "test",
		MinDelay: minDelay,
		MaxDelay: maxDelay,
		Match:    matchFunc,
	}
}

func alwaysMatch(_ error) bool { return true }
func neverMatch(_ error) bool  { return false }

func TestDelayingReconciler_ExponentialBackoff(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, alwaysMatch)

	inner := &fakeReconciler{
		err: ErrorWithCustomBackoff{
			error:          fmt.Errorf("throttled"),
			CustomBackoffs: []*CustomBackoff{cb},
		},
	}
	dr := newTestDelayingReconciler(inner)

	expectedDelays := []time.Duration{
		5 * time.Second,   // 5 * 2^0
		10 * time.Second,  // 5 * 2^1
		20 * time.Second,  // 5 * 2^2
		40 * time.Second,  // 5 * 2^3
		80 * time.Second,  // 5 * 2^4
		160 * time.Second, // 5 * 2^5
		320 * time.Second, // 5 * 2^6 = max
		320 * time.Second, // capped at max
	}

	for i, expected := range expectedDelays {
		result, err := dr.Reconcile(context.Background(), testRequest)
		assert.NoError(t, err, "iteration %d", i)
		assert.Equal(t, expected, result.RequeueAfter, "iteration %d", i)
	}
}

func TestComputeDelay(t *testing.T) {
	cases := []struct {
		name     string
		min, max time.Duration
		failures int
		expected time.Duration
	}{
		{"first failure", 5 * time.Second, 320 * time.Second, 1, 5 * time.Second},
		{"second failure", 5 * time.Second, 320 * time.Second, 2, 10 * time.Second},
		{"third failure", 5 * time.Second, 320 * time.Second, 3, 20 * time.Second},
		{"capped at max", 5 * time.Second, 320 * time.Second, 100, 320 * time.Second},
		{"zero failures", 5 * time.Second, 320 * time.Second, 0, 5 * time.Second},
		{"negative failures", 5 * time.Second, 320 * time.Second, -1, 5 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeDelay(tc.min, tc.max, tc.failures))
		})
	}
}

// --- Multiple CustomBackoffs: first match wins ---

func TestDelayingReconciler_MultipleCustomBackoffs_FirstMatchWins(t *testing.T) {
	// cb1: short delays (listed first — wins because first match wins)
	cb1 := newTestCustomBackoff(1*time.Second, 10*time.Second, alwaysMatch)
	cb1.Name = "short"
	// cb2: longer delays (listed second — skipped even though it matches)
	cb2 := newTestCustomBackoff(10*time.Second, 300*time.Second, alwaysMatch)
	cb2.Name = "long"

	inner := &fakeReconciler{
		err: ErrorWithCustomBackoff{
			error:          fmt.Errorf("error"),
			CustomBackoffs: []*CustomBackoff{cb1, cb2},
		},
	}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	// First match (cb1) wins — delay is 1s, not cb2's 10s
	assert.Equal(t, 1*time.Second, result.RequeueAfter)

	// Second failure: cb1 gives 2s (first match still wins)
	result, err = dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 2*time.Second, result.RequeueAfter)

	// Verify cb2's counter was never incremented
	key2 := backoffKey{cb: cb2, nn: testRequest.NamespacedName}
	dr.mu.Lock()
	_, exists := dr.counters[key2]
	dr.mu.Unlock()
	assert.False(t, exists, "cb2 counter should not exist since it was never matched")
}

// --- First match wins: only the first matching CB's Match() is called ---

func TestDelayingReconciler_FirstMatchWins_SkipsSubsequentMatch(t *testing.T) {
	cb1MatchCalls := 0
	cb2MatchCalls := 0

	cb1 := newTestCustomBackoff(1*time.Second, 10*time.Second, func(_ error) bool {
		cb1MatchCalls++
		return true
	})
	cb1.Name = "first"
	cb2 := newTestCustomBackoff(10*time.Second, 300*time.Second, func(_ error) bool {
		cb2MatchCalls++
		return true
	})
	cb2.Name = "second"

	inner := &fakeReconciler{
		err: ErrorWithCustomBackoff{
			error:          fmt.Errorf("error"),
			CustomBackoffs: []*CustomBackoff{cb1, cb2},
		},
	}
	dr := newTestDelayingReconciler(inner)

	dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, 1, cb1MatchCalls, "cb1 Match should be called once")
	assert.Equal(t, 0, cb2MatchCalls, "cb2 Match should never be called after cb1 matched")
}

// --- Counter clearing on success ---

func TestDelayingReconciler_SuccessClearsCounters(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, alwaysMatch)

	throttleErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("throttled"),
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: throttleErr}
	dr := newTestDelayingReconciler(inner)

	// Two failures — counter goes to 2.
	dr.Reconcile(context.Background(), testRequest)
	result, _ := dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, 10*time.Second, result.RequeueAfter) // 5 * 2^1

	// Success — counter should be cleared.
	inner.err = nil
	inner.result = reconcile.Result{}
	dr.Reconcile(context.Background(), testRequest)

	// Next failure should start fresh at minDelay.
	inner.err = throttleErr
	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter) // back to 5 * 2^0
}

// --- Counter clearing for non-matching CustomBackoff ---

func TestDelayingReconciler_NonMatchingBackoffResetsCounter(t *testing.T) {
	matchCount := 0
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, func(_ error) bool {
		matchCount++
		// Match on first two calls, then stop matching.
		return matchCount <= 2
	})

	throttleErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("error"),
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: throttleErr}
	dr := newTestDelayingReconciler(inner)

	// First two calls match — counter goes to 2.
	dr.Reconcile(context.Background(), testRequest)
	result, _ := dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, 10*time.Second, result.RequeueAfter) // 5 * 2^1

	// Third call: Match returns false — counter should be cleared, error passes through.
	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.Error(t, err) // non-matching means no delay, error passes through
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
	// Counter should have been deleted.
	key := backoffKey{cb: cb, nn: testRequest.NamespacedName}
	dr.mu.Lock()
	_, exists := dr.counters[key]
	dr.mu.Unlock()
	assert.False(t, exists, "counter should be cleared for non-matching backoff")
}

// --- Counter clearing on non-CustomBackoff error ---

func TestDelayingReconciler_NonCustomBackoffErrorClearsCounters(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, alwaysMatch)

	throttleErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("throttled"),
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: throttleErr}
	dr := newTestDelayingReconciler(inner)

	// Two failures — counter goes to 2.
	dr.Reconcile(context.Background(), testRequest)
	result, _ := dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, 10*time.Second, result.RequeueAfter) // 5 * 2^1

	// Non-CustomBackoff error — counters should be cleared.
	inner.err = fmt.Errorf("some other error")
	dr.Reconcile(context.Background(), testRequest)

	// Next CustomBackoff failure should start fresh at minDelay.
	inner.err = throttleErr
	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter) // back to 5 * 2^0
}

// --- CustomBackoff match clears OTHER counters ---

func TestDelayingReconciler_MatchClearsOtherCounters(t *testing.T) {
	cb1 := newTestCustomBackoff(1*time.Second, 100*time.Second, alwaysMatch)
	cb1.Name = "policy1"
	cb2 := newTestCustomBackoff(10*time.Second, 1000*time.Second, alwaysMatch)
	cb2.Name = "policy2"

	// Build up counter for cb1
	err1 := ErrorWithCustomBackoff{
		error:          fmt.Errorf("error1"),
		CustomBackoffs: []*CustomBackoff{cb1},
	}
	inner := &fakeReconciler{err: err1}
	dr := newTestDelayingReconciler(inner)

	dr.Reconcile(context.Background(), testRequest)
	dr.Reconcile(context.Background(), testRequest)
	// cb1 counter is now 2

	// Now reconcile with cb2 — cb1's counter should be cleared
	err2 := ErrorWithCustomBackoff{
		error:          fmt.Errorf("error2"),
		CustomBackoffs: []*CustomBackoff{cb2},
	}
	inner.err = err2
	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter) // cb2's minDelay

	// Verify cb1's counter was cleared
	key1 := backoffKey{cb: cb1, nn: testRequest.NamespacedName}
	dr.mu.Lock()
	_, exists := dr.counters[key1]
	dr.mu.Unlock()
	assert.False(t, exists, "cb1 counter should be cleared when cb2 matched")
}

// --- Independent tracking per CustomBackoff pointer ---

func TestDelayingReconciler_IndependentTrackingPerCustomBackoff(t *testing.T) {
	cb1 := newTestCustomBackoff(1*time.Second, 100*time.Second, alwaysMatch)
	cb1.Name = "policy1"
	cb2 := newTestCustomBackoff(10*time.Second, 1000*time.Second, alwaysMatch)
	cb2.Name = "policy2"

	// First: only cb1 triggers
	inner1 := &fakeReconciler{
		err: ErrorWithCustomBackoff{
			error:          fmt.Errorf("error1"),
			CustomBackoffs: []*CustomBackoff{cb1},
		},
	}
	dr := newTestDelayingReconciler(inner1)

	// Three failures with cb1
	dr.Reconcile(context.Background(), testRequest)
	dr.Reconcile(context.Background(), testRequest)
	result, _ := dr.Reconcile(context.Background(), testRequest)
	assert.Equal(t, 4*time.Second, result.RequeueAfter) // 1 * 2^2

	// Now switch to cb2 — should start at its own minDelay since it has separate tracking
	inner2 := &fakeReconciler{
		err: ErrorWithCustomBackoff{
			error:          fmt.Errorf("error2"),
			CustomBackoffs: []*CustomBackoff{cb2},
		},
	}
	dr.wrappedReconciler = inner2
	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 10*time.Second, result.RequeueAfter) // cb2's minDelay
}

// --- Independent tracking per NamespacedName ---

func TestDelayingReconciler_IndependentTrackingPerNamespacedName(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, alwaysMatch)

	throttleErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("throttled"),
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: throttleErr}
	dr := newTestDelayingReconciler(inner)

	req1 := reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns1", Name: "name1"},
	}
	req2 := reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: "ns2", Name: "name2"},
	}

	// Three failures on req1
	dr.Reconcile(context.Background(), req1)
	dr.Reconcile(context.Background(), req1)
	result1, _ := dr.Reconcile(context.Background(), req1)
	assert.Equal(t, 20*time.Second, result1.RequeueAfter) // 5 * 2^2

	// First failure on req2 — should be independent
	result2, err := dr.Reconcile(context.Background(), req2)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result2.RequeueAfter) // 5 * 2^0

	// req1 continues from where it left off
	result1, _ = dr.Reconcile(context.Background(), req1)
	assert.Equal(t, 40*time.Second, result1.RequeueAfter) // 5 * 2^3
}

// --- errors.As works through wrapping ---

func TestDelayingReconciler_ErrorsAsThroughWrapping(t *testing.T) {
	innerErr := &testThrottleError{msg: "throttled"}
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, func(err error) bool {
		var te *testThrottleError
		return errors.As(err, &te)
	})

	wrappedErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("wrapper: %w", innerErr),
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: wrappedErr}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
}

// --- errors.As finds ErrorWithCustomBackoff through wrapping ---

func TestDelayingReconciler_ErrorsAsFindsCustomBackoff(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, alwaysMatch)

	innerECB := ErrorWithCustomBackoff{
		error:          fmt.Errorf("throttled"),
		CustomBackoffs: []*CustomBackoff{cb},
	}
	// Wrap the ErrorWithCustomBackoff with fmt.Errorf %w
	wrappedErr := fmt.Errorf("outer: %w", innerECB)

	inner := &fakeReconciler{err: wrappedErr}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	assert.Equal(t, 5*time.Second, result.RequeueAfter)
}

// --- Verify the interface compliance ---

func TestDelayingReconciler_ImplementsReconciler(t *testing.T) {
	dr := NewDelayingReconciler(&fakeReconciler{}, log.WithField("test", true))
	require.NotNil(t, dr)
	_, ok := dr.(reconcile.Reconciler)
	assert.True(t, ok)
}

// --- No backoff when Match returns false for all policies ---

func TestDelayingReconciler_NoBackoffWhenNoMatch(t *testing.T) {
	cb := newTestCustomBackoff(5*time.Second, 320*time.Second, neverMatch)

	origErr := fmt.Errorf("some error")
	wrappedErr := ErrorWithCustomBackoff{
		error:          origErr,
		CustomBackoffs: []*CustomBackoff{cb},
	}

	inner := &fakeReconciler{err: wrappedErr}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	// Error passes through since no match
	assert.Error(t, err)
	assert.Equal(t, time.Duration(0), result.RequeueAfter)
}

// --- Multiple CustomBackoffs: only matching ones contribute ---

func TestDelayingReconciler_MultipleBackoffs_OnlyMatchingContribute(t *testing.T) {
	cbMatch := newTestCustomBackoff(10*time.Second, 300*time.Second, alwaysMatch)
	cbMatch.Name = "matching"
	cbNoMatch := newTestCustomBackoff(100*time.Second, 1000*time.Second, neverMatch)
	cbNoMatch.Name = "nonmatching"

	wrappedErr := ErrorWithCustomBackoff{
		error:          fmt.Errorf("error"),
		CustomBackoffs: []*CustomBackoff{cbMatch, cbNoMatch},
	}

	inner := &fakeReconciler{err: wrappedErr}
	dr := newTestDelayingReconciler(inner)

	result, err := dr.Reconcile(context.Background(), testRequest)
	assert.NoError(t, err)
	// Only cbMatch contributes (10s), cbNoMatch's 100s is NOT used
	assert.Equal(t, 10*time.Second, result.RequeueAfter)
}

package utils

import (
	"context"
	"errors"
	"math"
	"regexp"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const requeueDelay = time.Second * 1000

// CustomBackoff defines a backoff policy for a specific class of errors. Callers create
// package-level variables so the pointer identity can serve as part of the tracking key.
type CustomBackoff struct {
	// Name identifies this backoff policy in log messages.
	Name string
	// MinDelay is the initial (minimum) delay on the first failure.
	MinDelay time.Duration
	// MaxDelay is the upper bound for the exponential backoff.
	MaxDelay time.Duration
	// Match reports whether err should be subject to this backoff policy.
	Match func(error) bool
}

// ErrorWithCustomBackoff wraps an error with one or more CustomBackoff policies.
// When the DelayingReconciler sees this error, it checks each CustomBackoff's Match
// function and applies exponential backoff for the first match. If none match, the
// default backoff policy applies.
type ErrorWithCustomBackoff struct {
	error
	CustomBackoffs []*CustomBackoff
}

// Error satisfies the error interface, delegating to the wrapped error.
func (e ErrorWithCustomBackoff) Error() string {
	if e.error != nil {
		return e.error.Error()
	}
	return ""
}

// Unwrap returns the underlying error so errors.As / errors.Is work through the wrapper.
func (e ErrorWithCustomBackoff) Unwrap() error {
	return e.error
}

// NewErrorWithCustomBackoff creates an ErrorWithCustomBackoff wrapping err with the
// given backoff policies. Use this from external packages since the embedded error
// field is unexported.
func NewErrorWithCustomBackoff(err error, cbs ...*CustomBackoff) *ErrorWithCustomBackoff {
	return &ErrorWithCustomBackoff{
		error:          err,
		CustomBackoffs: cbs,
	}
}

// backoffKey identifies a (CustomBackoff policy, resource) pair for failure-count tracking.
type backoffKey struct {
	cb *CustomBackoff
	nn types.NamespacedName
}

type delayingReconciler struct {
	wrappedReconciler reconcile.Reconciler
	logger            log.FieldLogger

	mu       sync.Mutex
	counters map[backoffKey]int
}

// NewDelayingReconciler wraps a reconciler with additional error-handling logic:
//   - Errors matching the existing "forbidden" pattern (HTTP 401/403) trigger a fixed requeue delay.
//   - Errors wrapped in ErrorWithCustomBackoff trigger per-policy exponential backoff.
func NewDelayingReconciler(r reconcile.Reconciler, logger log.FieldLogger) reconcile.Reconciler {
	return &delayingReconciler{
		wrappedReconciler: r,
		logger:            logger,
		counters:          make(map[backoffKey]int),
	}
}

var forbiddenRegex = regexp.MustCompile(`\b40[13]\b`)

func isForbidden(err error) bool {
	if err == nil {
		return false
	}

	return forbiddenRegex.MatchString(err.Error())
}

func (d *delayingReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	result, err := d.wrappedReconciler.Reconcile(ctx, request)

	if isForbidden(err) {
		// HIVE-1895:
		// a) Do not consider this an error. Client credentials are bad.
		// b) Requeue and pray credentials are better next time
		d.logger.WithField("requeueAfter", requeueDelay).WithError(err).Info("Encountered permissions error. Requeueing request with delay.")
		return reconcile.Result{RequeueAfter: requeueDelay}, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	// Check for custom backoff policies. If the error is wrapped in ErrorWithCustomBackoff,
	// applies exponential backoff for the first match. If none match, the default backoff
	// policy applies.
	var matchedCB *CustomBackoff
	var ecb *ErrorWithCustomBackoff
	if errors.As(err, &ecb) {
		for _, cb := range ecb.CustomBackoffs {
			if matchedCB == nil && cb.Match(err) {
				matchedCB = cb
				key := backoffKey{cb: cb, nn: request.NamespacedName}
				d.counters[key]++
				delay := computeDelay(cb.MinDelay, cb.MaxDelay, d.counters[key])
				d.logger.WithFields(log.Fields{
					"requeueAfter": delay,
					"failureCount": d.counters[key],
					"backoff":      cb.Name,
				}).WithError(err).Info("Custom backoff triggered. Requeueing request with delay.")
				result = reconcile.Result{RequeueAfter: delay}
				err = nil
				break
			}
		}
	}

	// Clear all backoff counters for this resource except the matched one (if any).
	// On success (err == nil without ecb): matchedCB is nil, so all counters clear.
	// On a CustomBackoff match: only the winner's counter is preserved.
	// On non-CustomBackoff error: matchedCB is nil, so all counters clear.
	for key := range d.counters {
		if key.nn == request.NamespacedName && key.cb != matchedCB {
			delete(d.counters, key)
		}
	}

	return result, err
}

// computeDelay returns minDelay * 2^(failures-1), capped at maxDelay.
func computeDelay(minDelay, maxDelay time.Duration, failures int) time.Duration {
	if failures <= 0 {
		return minDelay
	}
	delay := time.Duration(float64(minDelay) * math.Pow(2, float64(failures-1)))
	if delay > maxDelay || delay <= 0 {
		// delay <= 0 catches overflow
		return maxDelay
	}
	return delay
}

var _ reconcile.Reconciler = &delayingReconciler{}

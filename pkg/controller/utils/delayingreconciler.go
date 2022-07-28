package utils

import (
	"context"
	log "github.com/sirupsen/logrus"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

const requeueDelay = time.Second * 1000

type delayingReconciler struct {
	wrappedReconciler reconcile.Reconciler
	logger            log.FieldLogger
}

func NewDelayingReconciler(r reconcile.Reconciler, logger log.FieldLogger) reconcile.Reconciler {
	return delayingReconciler{
		wrappedReconciler: r,
		logger:            logger,
	}
}

var forbiddenRegex = regexp.MustCompile(`\b40[13]\b`)

func isForbidden(err error) bool {
	if err == nil {
		return false
	}

	return forbiddenRegex.MatchString(err.Error())
}

func (d delayingReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	result, err := d.wrappedReconciler.Reconcile(ctx, request)

	if isForbidden(err) {
		// HIVE-1895:
		// a) Do not consider this an error. Client credentials are bad.
		// b) Requeue and pray credentials are better next time
		d.logger.WithField("requeueAfter", requeueDelay).WithError(err).Info("Encountered permissions error. Requeueing request with delay.")
		return reconcile.Result{RequeueAfter: requeueDelay}, nil
	}

	return result, err
}

var _ reconcile.Reconciler = &delayingReconciler{}

package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// NewRateLimitedUpdateEventHandler wraps the specified event handler inside a new
// event handler that will rate limit the incoming UPDATE events when the provided
// shouldRateLimit function returns true.
func NewRateLimitedUpdateEventHandler(eventHandler handler.EventHandler, shouldRateLimitFunc func(event.UpdateEvent) bool) handler.EventHandler {
	return &rateLimitedUpdateEventHandler{
		EventHandler:    eventHandler,
		shouldRateLimit: shouldRateLimitFunc,
	}
}

// rateLimitedUpdateEventHandler wraps the specified event handler such
// that it will rate limit the incoming UPDATE events when the provided
// shouldRateLimit function returns true.
type rateLimitedUpdateEventHandler struct {
	handler.EventHandler

	shouldRateLimit func(event.UpdateEvent) bool
}

var _ handler.EventHandler = &rateLimitedUpdateEventHandler{}

// Update implements handler.EventHandler
func (h *rateLimitedUpdateEventHandler) Update(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	nq := q
	if h.shouldRateLimit(e) {
		nq = &rateLimitedAddQueue{q}
	}
	h.EventHandler.Update(ctx, e, nq)
}

// rateLimitedAddQueue add queue wraps RateLimitingInterface queue
// such that the Add call also becomes rate limited.
type rateLimitedAddQueue struct {
	workqueue.TypedRateLimitingInterface[reconcile.Request]
}

var _ workqueue.TypedRateLimitingInterface[reconcile.Request] = &rateLimitedAddQueue{}

// Add implements workqueue.Interface
func (q *rateLimitedAddQueue) Add(item reconcile.Request) {
	q.TypedRateLimitingInterface.AddRateLimited(item)
}

// NewTypedRateLimitedUpdateEventHandler wraps the specified typed event handler inside a new
// event handler that will rate limit the incoming UPDATE events when the provided
// shouldRateLimit function returns true.
func NewTypedRateLimitedUpdateEventHandler[T runtime.Object, C comparable](typedEventHandler handler.TypedEventHandler[T, C], shouldRateLimitFunc func(event.UpdateEvent) bool) handler.TypedEventHandler[T, C] {
	return &typedRateLimitedUpdateEventHandler[T, C]{
		TypedEventHandler: typedEventHandler,
		shouldRateLimit:   shouldRateLimitFunc,
	}
}

// typedRateLimitedUpdateEventHandler wraps the specified typed event handler such
// that it will rate limit the incoming UPDATE events when the provided
// shouldRateLimit function returns true.
type typedRateLimitedUpdateEventHandler[T runtime.Object, C comparable] struct {
	handler.TypedEventHandler[T, C]

	shouldRateLimit func(event.UpdateEvent) bool
}

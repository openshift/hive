package utils

import (
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
)

// WrapEventHandlerWithLogging wraps the specified event handler inside a new
// event handler that will log when events are handled and items are added
// to the queue.
func WrapEventHandlerWithLogging(eventHandler handler.EventHandler, logger log.FieldLogger) handler.EventHandler {
	return &loggingEventHandler{
		logger:       logger,
		eventHandler: eventHandler,
	}
}

var _ handler.EventHandler = &loggingEventHandler{}

type loggingEventHandler struct {
	logger       log.FieldLogger
	eventHandler handler.EventHandler
}

// Create implements handler.EventHandler
func (h *loggingEventHandler) Create(e event.CreateEvent, q workqueue.RateLimitingInterface) {
	logger := h.loggerForEvent("create", e.Meta)
	logger.Debug("Handling event")
	h.eventHandler.Create(e, wrapQueueWithLogging(q, logger))
}

// Delete implements handler.EventHandler
func (h *loggingEventHandler) Delete(e event.DeleteEvent, q workqueue.RateLimitingInterface) {
	logger := h.loggerForEvent("delete", e.Meta)
	logger.Debug("Handling event")
	h.eventHandler.Delete(e, wrapQueueWithLogging(q, logger))
}

// Update implements handler.EventHandler
func (h *loggingEventHandler) Update(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
	logger := h.loggerForEvent("update", e.MetaNew)
	logger.Debug("Handling event")
	h.eventHandler.Update(e, wrapQueueWithLogging(q, logger))
}

// Generic implements handler.EventHandler
func (h *loggingEventHandler) Generic(e event.GenericEvent, q workqueue.RateLimitingInterface) {
	logger := h.loggerForEvent("generic", e.Meta)
	logger.Debug("Handling event")
	h.eventHandler.Generic(e, wrapQueueWithLogging(q, logger))
}

func (h *loggingEventHandler) loggerForEvent(eventType string, m metav1.Object) log.FieldLogger {
	return h.logger.
		WithField("event", eventType).
		WithField("name", m.GetName()).
		WithField("namespace", m.GetNamespace())
}

var _ inject.Scheme = &loggingEventHandler{}

// InjectScheme is called by the Controller to provide a singleton scheme to the event handler.
func (h *loggingEventHandler) InjectScheme(s *runtime.Scheme) error {
	_, err := inject.SchemeInto(s, h.eventHandler)
	return err
}

var _ inject.Mapper = &loggingEventHandler{}

// InjectMapper  is called by the Controller to provide the rest mapper used by the manager.
func (h *loggingEventHandler) InjectMapper(m meta.RESTMapper) error {
	_, err := inject.MapperInto(m, h.eventHandler)
	return err
}

func wrapQueueWithLogging(queue workqueue.RateLimitingInterface, logger log.FieldLogger) workqueue.RateLimitingInterface {
	return &loggingQueue{
		logger: logger,
		queue:  queue,
	}
}

var _ workqueue.RateLimitingInterface = &loggingQueue{}

type loggingQueue struct {
	logger log.FieldLogger
	queue  workqueue.RateLimitingInterface
}

// Add implements workqueue.Interface
func (q *loggingQueue) Add(item interface{}) {
	q.logger.Debugf("Adding %v", item)
	q.queue.Add(item)
}

// Len implements workqueue.Interface
func (q *loggingQueue) Len() int {
	return q.queue.Len()
}

// Get implements workqueue.Interface
func (q *loggingQueue) Get() (item interface{}, shutdown bool) {
	return q.queue.Get()
}

// Done implements workqueue.Interface
func (q *loggingQueue) Done(item interface{}) {
	q.queue.Done(item)
}

// ShutDown implements workqueue.Interface
func (q *loggingQueue) ShutDown() {
	q.queue.ShutDown()
}

// ShuttingDown implements workqueue.Interface
func (q *loggingQueue) ShuttingDown() bool {
	return q.queue.ShuttingDown()
}

// AddAfter implements workqueue.DelayingInterface
func (q *loggingQueue) AddAfter(item interface{}, duration time.Duration) {
	q.queue.AddAfter(item, duration)
}

// AddRateLimited implements workqueue.RateLimitingInterface
func (q *loggingQueue) AddRateLimited(item interface{}) {
	q.queue.AddRateLimited(item)
}

// Forget implements workqueue.RateLimitingInterface
func (q *loggingQueue) Forget(item interface{}) {
	q.queue.Forget(item)
}

// NumRequeues implements workqueue.RateLimitingInterface
func (q *loggingQueue) NumRequeues(item interface{}) int {
	return q.queue.NumRequeues(item)
}

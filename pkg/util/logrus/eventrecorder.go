package logrus

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/operator/events"
	log "github.com/sirupsen/logrus"
)

// loggingEventRecorder implements events.Recorder by logging to a logrus.FieldLogger
type loggingEventRecorder struct {
	logger    log.FieldLogger
	component string
}

// NewLoggingEventRecorder creates an event recorder that logs events via logrus
func NewLoggingEventRecorder(logger log.FieldLogger, component string) events.Recorder {
	return &loggingEventRecorder{
		logger:    logger,
		component: component,
	}
}

func (r *loggingEventRecorder) ComponentName() string {
	return r.component
}

func (r *loggingEventRecorder) ForComponent(component string) events.Recorder {
	// Return a new recorder with a modified logger and component
	return &loggingEventRecorder{
		logger:    r.logger.WithField("component", component),
		component: component,
	}
}

func (r *loggingEventRecorder) WithContext(ctx context.Context) events.Recorder {
	// Context doesn't affect logging, just return self
	return r
}

func (r *loggingEventRecorder) WithComponentSuffix(suffix string) events.Recorder {
	return r.ForComponent(fmt.Sprintf("%s-%s", r.component, suffix))
}

func (r *loggingEventRecorder) Shutdown() {}

func (r *loggingEventRecorder) Event(reason, message string) {
	r.logger.WithField("reason", reason).Debug(message)
}

func (r *loggingEventRecorder) Eventf(reason, messageFmt string, args ...interface{}) {
	r.Event(reason, fmt.Sprintf(messageFmt, args...))
}

func (r *loggingEventRecorder) Warning(reason, message string) {
	r.logger.WithField("reason", reason).Warning(message)
}

func (r *loggingEventRecorder) Warningf(reason, messageFmt string, args ...interface{}) {
	r.Warning(reason, fmt.Sprintf(messageFmt, args...))
}

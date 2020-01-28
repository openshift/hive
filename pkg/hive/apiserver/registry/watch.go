package registry

import (
	"sync"

	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"
)

type proxyWatcher struct {
	watcher   watch.Interface
	converter EventConverter
	result    chan watch.Event
	stopCh    chan struct{}
	mutex     sync.Mutex
	stopped   bool
	logger    log.FieldLogger
}

type EventConverter interface {
	Convert(watch.Event) (*watch.Event, error)
}

func NewProxyWatcher(watcher watch.Interface, converter EventConverter, logger log.FieldLogger) watch.Interface {
	pw := &proxyWatcher{
		watcher:   watcher,
		converter: converter,
		result:    make(chan watch.Event),
		stopCh:    make(chan struct{}),
		stopped:   false,
		logger:    logger,
	}
	go pw.receive()
	return pw
}

func (pw *proxyWatcher) Stop() {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()
	if !pw.stopped {
		pw.stopped = true
		close(pw.stopCh)
	}
	pw.watcher.Stop()
}

func (pw *proxyWatcher) ResultChan() <-chan watch.Event {
	return pw.result
}

func (pw *proxyWatcher) stopping() bool {
	pw.mutex.Lock()
	defer pw.mutex.Unlock()
	return pw.stopped
}

func (pw *proxyWatcher) receive() {
	defer close(pw.result)
	defer pw.Stop()
	for sourceEvent := range pw.watcher.ResultChan() {
		if pw.stopping() {
			return
		}
		if sourceEvent.Type == watch.Error {
			pw.result <- sourceEvent
			continue
		}
		resultEvent, err := pw.converter.Convert(sourceEvent)
		if err != nil {
			pw.logger.WithError(err).Error("could not convert event")
			status := apierrors.NewInternalError(err).Status()
			resultEvent = &watch.Event{
				Type:   watch.Error,
				Object: &status,
			}
		}
		pw.result <- *resultEvent
	}
}

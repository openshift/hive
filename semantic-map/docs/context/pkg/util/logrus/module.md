# Module atlas

## Responsibility

Provides adapters between logrus and two other logging interfaces: the `logr.Logger` interface (for controller-runtime) and the `events.Recorder` interface (for library-go operator events). Allows hive components to bridge logrus-based logging into these frameworks.

## Public Interface/API

- `NewLogr(logger log.FieldLogger) logr.Logger` -- returns a logr.Logger backed by logrus (Info maps to Debug, Error maps to Error)
- `NewLoggingEventRecorder(logger log.FieldLogger, component string) events.Recorder` -- returns an events.Recorder that logs events via logrus

## Internal Dependencies

- `github.com/go-logr/logr` -- logr.Logger interface
- `github.com/openshift/library-go/pkg/operator/events` -- events.Recorder interface
- `github.com/sirupsen/logrus` -- FieldLogger

## Capabilities

- Adapt logrus FieldLogger to logr.Logger (LogSink implementation)
- Adapt logrus FieldLogger to library-go events.Recorder
- Support component naming, context, and suffix on event recorder
- Key-value fields propagation from logr to logrus Fields

## Understanding Score

0.85

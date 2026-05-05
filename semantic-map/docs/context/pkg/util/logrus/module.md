# Module atlas

## Responsibility

Bridges Hive's preferred logging library (logrus) to two different interfaces used by the controller ecosystem:

1. **`logr.Logger` adapter** (`logr.go`) -- wraps a `logrus.FieldLogger` as a `logr.LogSink` so controller-runtime and other logr-consuming code can log through logrus. Info-level messages are routed to logrus Debug; Error-level messages go to logrus Error.
2. **`events.Recorder` adapter** (`eventrecorder.go`) -- implements the `library-go` `events.Recorder` interface by logging events via logrus instead of creating real Kubernetes Event objects. Useful in tests or contexts where no API server is available.

## Public Interface/API

- `NewLogr(logger log.FieldLogger) logr.Logger` -- returns a `logr.Logger` backed by the given logrus field logger. The underlying `lgr` sink implements `Init`, `Info`, `Error`, `Enabled`, `WithName`, and `WithValues`.
- `NewLoggingEventRecorder(logger log.FieldLogger, component string) events.Recorder` -- returns an `events.Recorder` that logs `Event`/`Eventf` at Debug level and `Warning`/`Warningf` at Warning level. Supports `ForComponent`, `WithComponentSuffix`, `WithContext`, `ComponentName`, and `Shutdown`.

## Internal Dependencies

- `context` -- for `WithContext` (no-op implementation).
- `fmt` -- string formatting in event recorder and component suffix.
- `github.com/go-logr/logr` -- target interface for the logr adapter.
- `github.com/openshift/library-go/pkg/operator/events` -- target interface for the event recorder adapter.
- `github.com/sirupsen/logrus` -- source logger implementation.

## Capabilities

- **`package`** name(s): **logrus**.
- Go **`import`** edges listed below (5 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/logrus`.
- Test file (`logr_test.go`) covers the logr adapter.

## Understanding Score

0.85

# Module atlas

## Responsibility

Test utility for creating logrus loggers with attached test hooks. Provides a factory for debug-level loggers and a helper to assert that a specific message was logged.

## Public Interface/API

- `NewLoggerWithHook() (*logrus.Logger, *logrustest.Hook)` -- creates a debug-level logger with a test hook attached for log entry inspection
- `AssertHookContainsMessage(t, hook, message) bool` -- asserts that at least one log entry in the hook matches the given message; returns true if message is empty

## Internal Dependencies

- `fmt`
- `github.com/sirupsen/logrus`
- `github.com/sirupsen/logrus/hooks/test`
- `github.com/stretchr/testify/assert`

## Capabilities

- **Package**: `logger`
- Not a builder package; provides test logging infrastructure

## Understanding Score

0.9

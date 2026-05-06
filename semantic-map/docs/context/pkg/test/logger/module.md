# Module atlas

## Responsibility

Test utility for creating logrus loggers with attached hooks and asserting that specific log messages were recorded.

## Public Interface/API

- `NewLoggerWithHook() (*logrus.Logger, *logrustest.Hook)` -- creates a debug-level logger with a test hook attached
- `AssertHookContainsMessage(t assert.TestingT, hook *logrustest.Hook, message string) bool` -- asserts that the hook captured an entry with the exact given message

## Internal Dependencies

- `github.com/sirupsen/logrus`
- `github.com/sirupsen/logrus/hooks/test`
- `github.com/stretchr/testify/assert`

## Capabilities

- Creates pre-configured debug-level logrus loggers for test capture
- Verifies expected log messages were emitted during test execution

## Understanding Score

0.9

package logger

import (
	"fmt"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

// NewLoggerWithHook creates a new logger with debug loglevel and attaches a hook to it.
func NewLoggerWithHook() (*logrus.Logger, *logrustest.Hook) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	hook := logrustest.NewLocal(logger)
	return logger, hook
}

func AssertHookContainsMessage(t assert.TestingT, hook *logrustest.Hook, message string) bool {
	if message == "" {
		return true
	}

	if hook == nil {
		return assert.Fail(t, "expect message but hook is nil")
	}

	for _, log := range hook.AllEntries() {
		if log.Message == message {
			return true
		}
	}

	return assert.Fail(t, fmt.Sprintf("%#v does not contain %#v", hook.Entries, message))
}

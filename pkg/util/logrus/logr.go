package logrus

import (
	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
)

// lgr uses Debug for info output and
// Error for error at all levels.
type lgr struct {
	logger log.FieldLogger
}

// NewLogr returns a new logger that implements logr.Logger interface
// using the FieldLogger.
func NewLogr(logger log.FieldLogger) logr.Logger {
	return logr.New(lgr{logger})
}

// Init implements logr.LogSink
func (lgr) Init(logr.RuntimeInfo) {}

// Info implements logr.LogSink
func (l lgr) Info(level int, msg string, keyAndValues ...any) {
	l.logger.WithFields(keyAndValuesToFields(keyAndValues...)).Debug(msg)
}

// Error implements logr.LogSink
func (l lgr) Error(err error, msg string, keyAndValues ...any) {
	l.logger.WithError(err).WithFields(keyAndValuesToFields(keyAndValues...)).Error(msg)
}

// Enabled implements logr.LogSink
func (lgr) Enabled(int) bool {
	return true
}

// WithName implements logr.LogSink
func (l lgr) WithName(name string) logr.LogSink {
	return lgr{logger: l.logger.WithField("_name", name)}
}

// WithValues implements logr.LogSink
func (l lgr) WithValues(keyAndValues ...any) logr.LogSink {
	return lgr{logger: l.logger.WithFields(keyAndValuesToFields(keyAndValues...))}
}

func keyAndValuesToFields(keyAndValues ...any) log.Fields {
	fields := log.Fields{}
	for idx := 0; idx < len(keyAndValues); {
		fields[keyAndValues[idx].(string)] = ""
		if idx+1 < len(keyAndValues) {
			fields[keyAndValues[idx].(string)] = keyAndValues[idx+1]
		}
		idx += 2
	}
	return fields
}

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
	return lgr{logger: logger}
}

var _ logr.Logger = lgr{}

// Info implements logr.InfoLogger
func (l lgr) Info(msg string, keyAndValues ...interface{}) {
	l.logger.WithFields(keyAndValuesToFields(keyAndValues...)).Debug(msg)
}

// Error implements logr.Logger
func (l lgr) Error(err error, msg string, keyAndValues ...interface{}) {
	l.logger.WithError(err).WithFields(keyAndValuesToFields(keyAndValues...)).Error(msg)
}

// Enabled implements logr.InfoLogger
func (lgr) Enabled() bool {
	return true
}

// V implements logr.Logger
func (l lgr) V(_ int) logr.InfoLogger {
	return l
}

// WithName implements logr.Logger
func (l lgr) WithName(name string) logr.Logger {
	return lgr{logger: l.logger.WithField("_name", name)}
}

// WithValues implements logr.Logger
func (l lgr) WithValues(keyAndValues ...interface{}) logr.Logger {
	return lgr{logger: l.logger.WithFields(keyAndValuesToFields(keyAndValues...))}
}

func keyAndValuesToFields(keyAndValues ...interface{}) log.Fields {
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

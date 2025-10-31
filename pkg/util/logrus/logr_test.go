package logrus

import (
	"bytes"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func Test_keyAndValuesToFields(t *testing.T) {
	cases := []struct {
		input  []any
		output log.Fields
	}{{
		input:  nil,
		output: log.Fields{},
	}, {
		input:  []any{},
		output: log.Fields{},
	}, {
		input:  []any{"key1", "value1", "key2", 1, "key3", 3.0, "key4", []int{1, 2}},
		output: log.Fields{"key1": "value1", "key2": 1, "key3": 3.0, "key4": []int{1, 2}},
	}, {
		input:  []any{"key1"},
		output: log.Fields{"key1": ""},
	}, {
		input:  []any{"key1", "value1", "key2"},
		output: log.Fields{"key1": "value1", "key2": ""},
	}}
	for _, test := range cases {
		fields := keyAndValuesToFields(test.input...)
		assert.Equal(t, test.output, fields)
	}
}

func Test_logr_debug(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)
	logger.SetFormatter(&log.TextFormatter{DisableColors: true, DisableTimestamp: true, DisableQuote: true})

	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	l := NewLogr(logger)
	testRun := func(l logr.Logger) {
		l.Info("first info message")
		l.Info("second info message with context", "key1", "value1", "key2", 10)

		l.Error(errors.New("error occurred"), "first error message")
		l.Error(errors.New("error occurred"), "second error message with context", "key1", "value1", "key2", 10)
	}

	testRun(l)

	lWithName := l.WithName("test-name")
	testRun(lWithName)

	lWithFields := l.WithValues("controller", "test-controller")
	testRun(lWithFields)

	lWithV2 := l.V(2)
	testRun(lWithV2)

	expected := `level=debug msg=first info message
level=debug msg=second info message with context key1=value1 key2=10
level=error msg=first error message error=error occurred
level=error msg=second error message with context error=error occurred key1=value1 key2=10
level=debug msg=first info message _name=test-name
level=debug msg=second info message with context _name=test-name key1=value1 key2=10
level=error msg=first error message _name=test-name error=error occurred
level=error msg=second error message with context _name=test-name error=error occurred key1=value1 key2=10
level=debug msg=first info message controller=test-controller
level=debug msg=second info message with context controller=test-controller key1=value1 key2=10
level=error msg=first error message controller=test-controller error=error occurred
level=error msg=second error message with context controller=test-controller error=error occurred key1=value1 key2=10
level=debug msg=first info message
level=debug msg=second info message with context key1=value1 key2=10
level=error msg=first error message error=error occurred
level=error msg=second error message with context error=error occurred key1=value1 key2=10
`
	assert.Equal(t, expected, buf.String())
}

func Test_logr_info(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.InfoLevel)
	logger.SetFormatter(&log.TextFormatter{DisableColors: true, DisableTimestamp: true, DisableQuote: true})

	buf := &bytes.Buffer{}
	logger.SetOutput(buf)
	l := NewLogr(logger)
	testRun := func(l logr.Logger) {
		l.Info("first info message")
		l.Info("second info message with context", "key1", "value1", "key2", 10)

		l.Error(errors.New("error occurred"), "first error message")
		l.Error(errors.New("error occurred"), "second error message with context", "key1", "value1", "key2", 10)
	}

	testRun(l)

	lWithName := l.WithName("test-name")
	testRun(lWithName)

	lWithFields := l.WithValues("controller", "test-controller")
	testRun(lWithFields)

	lWithV2 := l.V(2)
	testRun(lWithV2)

	expected := `level=error msg=first error message error=error occurred
level=error msg=second error message with context error=error occurred key1=value1 key2=10
level=error msg=first error message _name=test-name error=error occurred
level=error msg=second error message with context _name=test-name error=error occurred key1=value1 key2=10
level=error msg=first error message controller=test-controller error=error occurred
level=error msg=second error message with context controller=test-controller error=error occurred key1=value1 key2=10
level=error msg=first error message error=error occurred
level=error msg=second error message with context error=error occurred key1=value1 key2=10
`
	assert.Equal(t, expected, buf.String())
}

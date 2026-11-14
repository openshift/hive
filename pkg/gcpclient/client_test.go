package gcpclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/api/googleapi"
)

func TestIsResourceInUseError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		expect bool
	}{{
		name:   "nil error",
		err:    nil,
		expect: false,
	}, {
		name:   "operation error with resource-in-use code",
		err:    &OperationError{Code: ResourceInUseReason},
		expect: true,
	}, {
		name:   "operation error with other code",
		err:    &OperationError{Code: "QUOTA_EXCEEDED"},
		expect: false,
	}, {
		name:   "googleapi error with resource-in-use reason",
		err:    &googleapi.Error{Code: 400, Errors: []googleapi.ErrorItem{{Reason: ResourceInUseReason}}},
		expect: true,
	}, {
		name:   "googleapi error with other reason",
		err:    &googleapi.Error{Code: 401, Errors: []googleapi.ErrorItem{{Reason: "accessDenied"}}},
		expect: false,
	}, {
		name:   "wrapped operation error",
		err:    errors.New("wrapper: " + (&OperationError{Code: ResourceInUseReason}).Error()),
		expect: false, // plain string wrapper is not typed; discrimination is intentionally type-based
	}, {
		name:   "unrelated error",
		err:    errors.New("boom"),
		expect: false,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expect, IsResourceInUseError(test.err))
		})
	}
}

func TestOperationErrorMessage(t *testing.T) {
	err := &OperationError{Code: ResourceInUseReason, Message: "subnet in use"}
	assert.Equal(t, "operation completed with error: RESOURCE_IN_USE_BY_OTHER_RESOURCE", err.Error())
}

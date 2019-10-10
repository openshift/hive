package utils

import (
	"testing"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestMergeJsons(t *testing.T) {
	tests := []struct {
		name        string
		jsonStr1    string
		jsonStr2    string
		expectedStr string
		expectedErr bool
	}{
		{
			name:        "Merge pull secrets 01",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"quay.io":{"auth":"b3BlbnNoVkc=","email":"abc@xyz.com"},"registry.connect.redhat.com":{"auth":"NjQ4ODeDZ3d1pN","email":"abc@xyz.com"},"registry.redhat.io":{"auth":"NjQ4ODX1pN","email":"abc@xyz.com"},"registry.svc.ci.okd.org":{"auth":"dXNljlfjldsfSDD"}}}`,
		},
		{
			name:        "Merge pull secrets 02",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"},"registry.svc.ci.okd.org":{"auth":"dXNljlfjldsfSDD"}}}`,
		},
		{
			name:        "Merege global and local same auth key but different secret",
			jsonStr1:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTlocal=","email":"abc@xyz.com"}}}`,
			expectedStr: `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTlocal=","email":"abc@xyz.com"}}}`,
		},
		{
			name:        "Merge of pull secrets should fail",
			jsonStr1:    `{"auths": {"registry.svc.ci.okd.org": {"auth": "dXNljlfjldsfSDD"}}}`,
			jsonStr2:    `{"auths":{"cloud.okd.com":{"auth":"b34xVjWERckjfUyV1pMQTc=","email":"abc@xyz.com"}}`,
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tLogger := log.New()
			resultStr, err := MergeJsons(test.jsonStr1, test.jsonStr2, tLogger)
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				assert.Equal(t, test.expectedStr, resultStr)
			}
		})
	}

}

func TestLogLevel(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		expectedLevel log.Level
	}{
		{
			name:          "nil",
			err:           nil,
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "already exists",
			err:           apierrors.NewAlreadyExists(schema.GroupResource{}, ""),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "conflict",
			err:           apierrors.NewConflict(schema.GroupResource{}, "", nil),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "other api error",
			err:           apierrors.NewUnauthorized(""),
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "wrapped info-level error",
			err:           errors.Wrap(apierrors.NewAlreadyExists(schema.GroupResource{}, ""), "wrapper"),
			expectedLevel: log.InfoLevel,
		},
		{
			name:          "wrapped error-level error",
			err:           errors.Wrap(apierrors.NewUnauthorized(""), "wrapper"),
			expectedLevel: log.ErrorLevel,
		},
		{
			name:          "double-wrapped error",
			err:           errors.Wrap(errors.Wrap(apierrors.NewAlreadyExists(schema.GroupResource{}, ""), "inner wrapper"), "outer wrapper"),
			expectedLevel: log.InfoLevel,
		},
	}
	for _, tc := range cases {
		actualLevel := LogLevel(tc.err)
		assert.Equal(t, tc.expectedLevel, actualLevel)
	}
}

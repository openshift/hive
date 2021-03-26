package installmanager

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewCredentialProcessResponse(t *testing.T) {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)

	cases := []struct {
		name  string
		creds credentials.Value
		resp  string
	}{{
		name: "valid credentials",
		creds: credentials.Value{
			AccessKeyID:     "ASX..ID...",
			SecretAccessKey: "ASX..SECRET...",
			SessionToken:    "ASX..TOKEN...",
		},
		resp: `{"Version":1,"AccessKeyId":"ASX..ID...","SecretAccessKey":"ASX..SECRET...","SessionToken":"ASX..TOKEN...","Expiration":"0001-01-01T00:00:00Z"}`,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := newCredentialProcessResponse(test.creds, time.Time{})
			require.NoError(t, err)
			assert.Equal(t, test.resp, got)
		})
	}
}

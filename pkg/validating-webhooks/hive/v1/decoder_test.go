package v1

import (
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

func createDecoder(t *testing.T) *admission.Decoder {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	decoder, err := admission.NewDecoder(scheme)
	require.NoError(t, err, "unexpected error creating decoder")
	return decoder
}

package v1

import (
	"testing"

	"github.com/openshift/hive/pkg/util/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func createDecoder(t *testing.T) *admission.Decoder {
	scheme := scheme.GetScheme()
	decoder := admission.NewDecoder(scheme)
	return decoder
}

package v1

import (
	"github.com/openshift/hive/pkg/util/scheme"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func createDecoder() *admission.Decoder {
	scheme := scheme.GetScheme()
	decoder := admission.NewDecoder(scheme)
	return &decoder
}

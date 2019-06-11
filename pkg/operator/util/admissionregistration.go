package util

import (
	admregv1 "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// ReadValidatingWebhookConfigurationV1Beta1OrDie reads a ValidatingWebhookConfiguration,
// as this is not yet added to library-go.
func ReadValidatingWebhookConfigurationV1Beta1OrDie(objBytes []byte, scheme *runtime.Scheme) *admregv1.ValidatingWebhookConfiguration {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(admregv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*admregv1.ValidatingWebhookConfiguration)
}

// ReadMutatingWebhookConfigurationV1Beta1OrDie reads a MutatingWebhookConfiguration,
// as this is not yet added to library-go.
func ReadMutatingWebhookConfigurationV1Beta1OrDie(objBytes []byte, scheme *runtime.Scheme) *admregv1.MutatingWebhookConfiguration {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(admregv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*admregv1.MutatingWebhookConfiguration)
}

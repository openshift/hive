package util

import (
	admregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

// ReadValidatingWebhookConfigurationV1OrDie reads a ValidatingWebhookConfiguration,
// as this is not yet added to library-go.
func ReadValidatingWebhookConfigurationV1OrDie(objBytes []byte) *admregv1.ValidatingWebhookConfiguration {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme.Scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(admregv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*admregv1.ValidatingWebhookConfiguration)
}

// ReadAPIServiceV1Beta1OrDie reads an APIService, as this is not yet added to library-go.
func ReadAPIServiceV1Beta1OrDie(objBytes []byte) *apiregistrationv1.APIService {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme.Scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(apiregistrationv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*apiregistrationv1.APIService)
}

func ReadEndpointsV1OrDie(objBytes []byte) *corev1.Endpoints {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme.Scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(corev1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*corev1.Endpoints)
}

func ReadDeploymentV1OrDie(objBytes []byte) *appsv1.Deployment {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme.Scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(appsv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*appsv1.Deployment)
}

func ReadServiceV1OrDie(objBytes []byte) *corev1.Service {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme.Scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(corev1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*corev1.Service)
}

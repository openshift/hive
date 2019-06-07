package util

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
)

// ReadAPIServiceV1Beta1OrDie reads an APIService, as this is not yet added to library-go.
func ReadAPIServiceV1Beta1OrDie(objBytes []byte, scheme *runtime.Scheme) *apiregistrationv1.APIService {
	apiExtensionsCodecs := serializer.NewCodecFactory(scheme)

	requiredObj, err := runtime.Decode(apiExtensionsCodecs.UniversalDecoder(apiregistrationv1.SchemeGroupVersion), objBytes)
	if err != nil {
		panic(err)
	}
	return requiredObj.(*apiregistrationv1.APIService)
}

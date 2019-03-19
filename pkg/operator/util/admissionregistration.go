/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

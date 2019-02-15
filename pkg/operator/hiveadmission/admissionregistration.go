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
package hiveadmission

import (
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	admregv1 "k8s.io/api/admissionregistration/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	admregv1client "k8s.io/client-go/kubernetes/typed/admissionregistration/v1beta1"
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

// ApplyValidatingWebhookConfiguration merges objectmeta and requires apiservice coordinates.  It does not touch CA bundles, which should be managed via service CA controller.
func ApplyValidatingWebhookConfiguration(client admregv1client.AdmissionregistrationV1beta1Interface, required *admregv1.ValidatingWebhookConfiguration) (*admregv1.ValidatingWebhookConfiguration, bool, error) {
	existing, err := client.ValidatingWebhookConfigurations().Get(required.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		actual, err := client.ValidatingWebhookConfigurations().Create(required)
		return actual, true, err
	}
	if err != nil {
		return nil, false, err
	}

	modified := resourcemerge.BoolPtr(false)
	resourcemerge.EnsureObjectMeta(modified, &existing.ObjectMeta, required.ObjectMeta)
	if equality.Semantic.DeepEqual(existing.Webhooks, required.Webhooks) {
		return existing, false, nil
	}

	existing.Webhooks = required.Webhooks

	actual, err := client.ValidatingWebhookConfigurations().Update(existing)
	return actual, true, err
}

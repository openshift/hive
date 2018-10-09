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

package validatingwebhooks

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"strings"

	"net/http"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	dnsvalidation "k8s.io/apimachinery/pkg/util/validation"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

const (
	dnsZoneGroup    = "hive.openshift.io"
	dnsZoneVersion  = "v1alpha1"
	dnsZoneResource = "dnszones"
)

// DNSZoneValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type DNSZoneValidatingAdmissionHook struct{}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1alpha1/dnszones".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *DNSZoneValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "dnszones",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the DNSZone CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1alpha1",
			Resource: "dnszones",
		},
		"dnszone"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *DNSZoneValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "dnszones",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *DNSZoneValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "Validate",
	})

	if !a.shouldValidate(admissionSpec) {
		contextLogger.Info("Skipping validation for request")
		// The request object isn't something that this validator should validate.
		// Therefore, we say that it's valid.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	contextLogger.Info("Validating request")

	if admissionSpec.Operation == admissionv1beta1.Create {
		return a.validateCreate(admissionSpec)
	}

	if admissionSpec.Operation == admissionv1beta1.Update {
		return a.validateUpdate(admissionSpec)
	}

	// We should never get here.
	contextLogger.Error("Not Implemented. Should not get here.")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: false,
	}
}

func (a *DNSZoneValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != dnsZoneGroup {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != dnsZoneVersion {
		contextLogger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != dnsZoneResource {
		contextLogger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	if admissionSpec.Operation != admissionv1beta1.Create &&
		admissionSpec.Operation != admissionv1beta1.Update {
		contextLogger.Debug("Returning False, we only validate creates and updates. Nothing else.")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for DNSZone objects.
func (a *DNSZoneValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	newObject := &hivev1.DNSZone{}
	err := json.Unmarshal(admissionSpec.Object.Raw, newObject)
	if err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	if newObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["object.Name"] = newObject.Name
	}

	strErrs := dnsvalidation.IsDNS1123Subdomain(newObject.Spec.Zone)
	if len(strErrs) != 0 {
		message := fmt.Sprintf("Failed validation: %v", strings.Join(strErrs, ";"))
		contextLogger.Infof(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateUpdate specifically validates update operations for DNSZone objects.
func (a *DNSZoneValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	newObject := &hivev1.DNSZone{}
	err := json.Unmarshal(admissionSpec.Object.Raw, newObject)
	if err != nil {
		contextLogger.Errorf("Failed unmarshaling Object: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	if newObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["object.Name"] = newObject.Name
	}

	oldObject := &hivev1.DNSZone{}
	err = json.Unmarshal(admissionSpec.OldObject.Raw, oldObject)
	if err != nil {
		contextLogger.Errorf("Failed unmarshaling OldObject: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	if oldObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["oldObject.Name"] = oldObject.Name
	}

	if oldObject.Spec.Zone != newObject.Spec.Zone {
		message := "DNSZone.Spec.Zone is immutable"
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

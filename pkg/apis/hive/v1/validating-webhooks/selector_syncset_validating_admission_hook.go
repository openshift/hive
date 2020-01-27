package validatingwebhooks

import (
	"encoding/json"

	log "github.com/sirupsen/logrus"

	"net/http"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
)

const (
	selectorSyncSetGroup    = "hive.openshift.io"
	selectorSyncSetVersion  = "v1"
	selectorSyncSetResource = "selectorsyncsets"
)

// SelectorSyncSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type SelectorSyncSetValidatingAdmissionHook struct{}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/selectorsyncsetvalidators".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *SelectorSyncSetValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "selectorsyncsetvalidator",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the SelectorSyncSet CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1",
			Resource: "selectorsyncsetvalidators",
		},
		"selectorsyncsetvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *SelectorSyncSetValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "selectorsyncsetvalidator",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *SelectorSyncSetValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
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
		// Therefore, we say that it's allowed.
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

	// We're only validating creates and updates at this time, so all other operations are explicitly allowed.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// shouldValidate explicitly checks if the request should validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *SelectorSyncSetValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != selectorSyncSetGroup {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != selectorSyncSetVersion {
		contextLogger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != selectorSyncSetResource {
		contextLogger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for SelectorSyncSet objects.
func (a *SelectorSyncSetValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	newObject := &hivev1.SelectorSyncSet{}
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

	// Add the new data to the contextLogger
	contextLogger.Data["object.Name"] = newObject.Name

	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateResources(newObject.Spec.Resources, field.NewPath("spec").Child("resources"))...)
	allErrs = append(allErrs, validatePatches(newObject.Spec.Patches, field.NewPath("spec").Child("patches"))...)
	allErrs = append(allErrs, validateSecrets(newObject.Spec.Secrets, field.NewPath("spec").Child("secretMappings"))...)

	if len(allErrs) > 0 {
		statusError := errors.NewInvalid(newObject.GroupVersionKind().GroupKind(), newObject.Name, allErrs).Status()
		contextLogger.Infof(statusError.Message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &statusError,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateUpdate specifically validates update operations for SelectorSyncSet objects.
func (a *SelectorSyncSetValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	newObject := &hivev1.SelectorSyncSet{}
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

	// Add the new data to the contextLogger
	contextLogger.Data["object.Name"] = newObject.Name

	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateResources(newObject.Spec.Resources, field.NewPath("spec", "resources"))...)
	allErrs = append(allErrs, validatePatches(newObject.Spec.Patches, field.NewPath("spec", "patches"))...)
	allErrs = append(allErrs, validateSecrets(newObject.Spec.Secrets, field.NewPath("spec", "secretReferences"))...)

	if len(allErrs) > 0 {
		statusError := errors.NewInvalid(newObject.GroupVersionKind().GroupKind(), newObject.Name, allErrs).Status()
		contextLogger.Infof(statusError.Message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &statusError,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

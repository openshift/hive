package validatingwebhooks

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"net/http"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
)

const (
	syncSetGroup    = "hive.openshift.io"
	syncSetVersion  = "v1alpha1"
	syncSetResource = "syncsets"
)

// SyncSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type SyncSetValidatingAdmissionHook struct{}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1alpha1/syncsets".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *SyncSetValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "syncsets",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the SyncSet CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1alpha1",
			Resource: "syncsets",
		},
		"syncset"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *SyncSetValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "syncsets",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *SyncSetValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
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
func (a *SyncSetValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != syncSetGroup {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != syncSetVersion {
		contextLogger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != syncSetResource {
		contextLogger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for SyncSet objects.
func (a *SyncSetValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	newObject := &hivev1.SyncSet{}
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

	if invalid, ok := checkValidPatchTypes(newObject.Spec.Patches); !ok {
		message := fmt.Sprintf("Failed validation: Invalid patch type detected: %s. Valid patch types are: json, merge, and strategic", invalid)
		contextLogger.Infof(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonInvalid,
				Message: message,
			},
		}
	}

	if allErrs := validateSecretReferences(newObject.Spec.SecretReferences, field.NewPath("spec").Child("secretReferences")); len(allErrs) > 0 {
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

// validateUpdate specifically validates update operations for SyncSet objects.
func (a *SyncSetValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	newObject := &hivev1.SyncSet{}
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

	if invalid, ok := checkValidPatchTypes(newObject.Spec.Patches); !ok {
		message := fmt.Sprintf("Failed validation: Invalid patch type detected: %s. Valid patch types are: json, merge, and strategic", invalid)
		contextLogger.Infof(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonInvalid,
				Message: message,
			},
		}
	}

	if allErrs := validateSecretReferences(newObject.Spec.SecretReferences, field.NewPath("spec").Child("secretReferences")); len(allErrs) > 0 {
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

func checkValidPatchTypes(patches []hivev1.SyncObjectPatch) (string, bool) {
	validTypes := map[string]struct{}{"json": {}, "merge": {}, "strategic": {}}
	for _, patch := range patches {
		if _, ok := validTypes[patch.PatchType]; !ok {
			return patch.PatchType, false
		}
	}
	return "", true
}

func validateSecretReferences(secrets []hivev1.SecretReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, secret := range secrets {
		allErrs = append(allErrs, validateSecretRef(secret.Source, fldPath.Index(i).Child("source"))...)
		allErrs = append(allErrs, validateSecretRef(secret.Target, fldPath.Index(i).Child("target"))...)
	}
	return allErrs
}

func validateSecretRef(ref corev1.ObjectReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ref.Kind) > 0 && ref.Kind != "secret" {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("kind"), ref.Kind, []string{"secret"}))
	}
	if ref.GroupVersionKind().Group != "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("apiVersion"), ref.APIVersion, "Group part of API version must be empty"))
	}
	if len(ref.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "Name is required"))
	}
	if len(ref.FieldPath) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("fieldPath"), ref.FieldPath, "FieldPath should not be set"))
	}
	if len(ref.UID) != 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("UID"), ref.UID, "UID should not be set"))
	}
	return allErrs
}

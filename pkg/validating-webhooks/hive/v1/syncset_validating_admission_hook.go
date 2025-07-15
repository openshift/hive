package v1

import (
	"encoding/json"
	"net/http"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	syncSetGroup    = "hive.openshift.io"
	syncSetVersion  = "v1"
	syncSetResource = "syncsets"
)

var invalidResourceGroupKinds = map[string]map[string]bool{
	"authorization.openshift.io": {
		"Role":                true,
		"RoleBinding":         true,
		"ClusterRole":         true,
		"ClusterRoleBinding":  true,
		"SubjectAccessReview": true,
	},
}

// HIVE-2807: "" is defaulted to "strategic" in code.
var validPatchTypes = sets.New("", "json", "merge", "strategic")

var (
	validResourceApplyModes = map[hivev1.SyncSetResourceApplyMode]bool{
		hivev1.UpsertResourceApplyMode: true,
		hivev1.SyncResourceApplyMode:   true,
	}

	validResourceApplyModeSlice = func() []string {
		v := make([]string, 0, len(validResourceApplyModes))
		for m := range validResourceApplyModes {
			v = append(v, string(m))
		}
		return v
	}()
)

// SyncSetValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type SyncSetValidatingAdmissionHook struct {
	decoder admission.Decoder
}

// NewSyncSetValidatingAdmissionHook constructs a new SyncSetValidatingAdmissionHook
func NewSyncSetValidatingAdmissionHook(decoder admission.Decoder) *SyncSetValidatingAdmissionHook {
	return &SyncSetValidatingAdmissionHook{decoder: decoder}
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/syncsetvalidators".
// When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *SyncSetValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "syncsetvalidator",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the SyncSet CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1",
			Resource: "syncsetvalidators",
		},
		"syncsetvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *SyncSetValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1",
		"resource": "syncsetvalidator",
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
	if err := a.decoder.DecodeRaw(admissionSpec.Object, newObject); err != nil {
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
	allErrs = append(allErrs, validateSourceSecretInSyncSetNamespace(newObject.Spec.Secrets, newObject.Namespace, field.NewPath("spec", "secretMappings"))...)
	allErrs = append(allErrs, validateResourceApplyMode(newObject.Spec.ResourceApplyMode, field.NewPath("spec", "resourceApplyMode"))...)

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
	if err := a.decoder.DecodeRaw(admissionSpec.Object, newObject); err != nil {
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
	allErrs = append(allErrs, validateSecrets(newObject.Spec.Secrets, field.NewPath("spec", "secretMappings"))...)
	allErrs = append(allErrs, validateSourceSecretInSyncSetNamespace(newObject.Spec.Secrets, newObject.Namespace, field.NewPath("spec", "secretMappings"))...)
	allErrs = append(allErrs, validateResourceApplyMode(newObject.Spec.ResourceApplyMode, field.NewPath("spec", "resourceApplyMode"))...)

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

func validatePatches(patches []hivev1.SyncObjectPatch, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, patch := range patches {
		if !validPatchTypes.Has(patch.PatchType) {
			allErrs = append(allErrs, field.NotSupported(fldPath.Index(i).Child("PatchType"), patch.PatchType, sets.List(validPatchTypes)))
		}
	}
	return allErrs
}

func validateResourceApplyMode(resourceApplyMode hivev1.SyncSetResourceApplyMode, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if resourceApplyMode != "" && !validResourceApplyModes[resourceApplyMode] {
		allErrs = append(allErrs, field.NotSupported(fldPath, resourceApplyMode, validResourceApplyModeSlice))
	}
	return allErrs
}

func validateResources(resources []runtime.RawExtension, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, resource := range resources {
		allErrs = append(allErrs, validateResource(resource, fldPath.Index(i))...)
	}
	return allErrs
}

func validateResource(resource runtime.RawExtension, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	u := &unstructured.Unstructured{}
	err := json.Unmarshal(resource.Raw, u)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath, resource.Raw, "Unable to unmarshal resource"))
		return allErrs
	}

	if invalidResourceGroupKinds[u.GroupVersionKind().Group][u.GetKind()] {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("APIVersion"), u.GetAPIVersion(), "must use kubernetes group for this resource kind"))
	}

	return allErrs
}

func validateSecrets(secrets []hivev1.SecretMapping, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, secret := range secrets {
		allErrs = append(allErrs, validateSecretRef(secret.SourceRef, fldPath.Index(i).Child("sourceRef"))...)
		allErrs = append(allErrs, validateSecretRef(secret.TargetRef, fldPath.Index(i).Child("targetRef"))...)
	}
	return allErrs
}

func validateSourceSecretInSyncSetNamespace(secrets []hivev1.SecretMapping, syncSetNS string, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	for i, secret := range secrets {
		if secret.SourceRef.Namespace != syncSetNS && secret.SourceRef.Namespace != "" {
			path := fldPath.Index(i).Child("sourceRef")

			allErrs = append(allErrs, field.Invalid(path.Child("namespace"), secret.SourceRef.Namespace,
				"source secret reference must be in same namespace as SyncSet"))
		}
	}
	return allErrs
}

func validateSecretRef(ref hivev1.SecretReference, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(ref.Name) == 0 {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "Name is required"))
	}
	return allErrs
}

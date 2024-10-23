package v1

import (
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	clusterDeploymentCustomizationGroup    = "hive.openshift.io"
	clusterDeploymentCustomizationVersion  = "v1"
	clusterDeploymentCustomizationResource = "clusterdeploymentcustomization"

	clusterDeploymentCustomizationAdmissionGroup   = "admission.hive.openshift.io"
	clusterDeploymentCustomizationAdmissionVersion = "v1"
)

// ClusterDeploymentCustomizationlValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterDeploymentCustomizationValidatingAdmissionHook struct {
	decoder admission.Decoder
}

// NewClusterDeploymentCustomizationValidatingAdmissionHook constructs a new ClusterDeploymentCustomizationValidatingAdmissionHook
func NewClusterDeploymentCustomizationValidatingAdmissionHook(decoder admission.Decoder) *ClusterDeploymentCustomizationValidatingAdmissionHook {
	return &ClusterDeploymentCustomizationValidatingAdmissionHook{
		decoder: decoder,
	}
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/clusterdeploymentcustomizationvalidators".
// When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentCustomizationAdmissionGroup,
		"version":  clusterDeploymentCustomizationAdmissionVersion,
		"resource": "clusterdeploymentcustomizationvalidator",
	}).Info("Registering validation REST resource")

	// NOTE: This GVR is meant to be different than the ClusterDeploymentCustomization CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    clusterDeploymentCustomizationAdmissionGroup,
			Version:  clusterDeploymentCustomizationAdmissionVersion,
			Resource: "clusterdeploymentcustomizationvalidators",
		},
		"clusterdeploymentcustomizationvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentCustomizationAdmissionGroup,
		"version":  clusterDeploymentCustomizationAdmissionVersion,
		"resource": "clusterdeploymentcustomizationvalidator",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
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
		// Therefore, we say that it's Allowed.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	contextLogger.Info("Validating request")

	switch admissionSpec.Operation {
	case admissionv1beta1.Create:
		return a.validateCreate(admissionSpec)
	case admissionv1beta1.Update:
		return a.validateUpdate(admissionSpec)
	default:
		contextLogger.Info("Successful validation")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}

// shouldValidate explicitly checks if the request should validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != clusterDeploymentCustomizationGroup {
		contextLogger.Info("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != clusterDeploymentCustomizationVersion {
		contextLogger.Info("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != clusterDeploymentCustomizationResource {
		contextLogger.Info("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for ClusterDeploymentCustomization objects.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	cdc := &hivev1.ClusterDeploymentCustomization{}
	if err := a.decoder.DecodeRaw(admissionSpec.Object, cdc); err != nil {
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
	contextLogger.Data["object.Name"] = cdc.Name

	// TODO: Put Create Validation Here (or in openAPIV3Schema validation section of crd)

	if len(cdc.Name) > validation.DNS1123LabelMaxLength {
		message := fmt.Sprintf("Invalid cluster deployment customization name (.meta.name): %s", validation.MaxLenError(validation.DNS1123LabelMaxLength))
		contextLogger.Error(message)
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

// validateUpdate specifically validates update operations for ClusterDeployment objects.
func (a *ClusterDeploymentCustomizationValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	newObject := &hivev1.ClusterDeploymentCustomization{}
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

	oldObject := &hivev1.ClusterDeploymentCustomization{}
	if err := a.decoder.DecodeRaw(admissionSpec.OldObject, oldObject); err != nil {
		contextLogger.Errorf("Failed unmarshaling OldObject: %v", err.Error())
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}

	// Add the new data to the contextLogger
	contextLogger.Data["oldObject.Name"] = oldObject.Name

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

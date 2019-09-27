package validatingwebhooks

import (
	"fmt"

	log "github.com/sirupsen/logrus"

	"net/http"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"
)

const (
	clusterProvisionGroup    = "hive.openshift.io"
	clusterProvisionVersion  = "v1alpha1"
	clusterProvisionResource = "clusterprovisions"
)

var (
	validProvisionStages = map[hivev1.ClusterProvisionStage]bool{
		hivev1.ClusterProvisionStageInitializing: true,
		hivev1.ClusterProvisionStageProvisioning: true,
		hivev1.ClusterProvisionStageComplete:     true,
		hivev1.ClusterProvisionStageFailed:       true,
	}

	validProvisionStageValues = func() []string {
		v := make([]string, 0, len(validProvisionStages))
		for m := range validProvisionStages {
			v = append(v, string(m))
		}
		return v
	}()
)

// ClusterProvisionValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterProvisionValidatingAdmissionHook struct {
	decoder runtime.Decoder
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
// webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1alpha1/clusterprovisionvalidators".
// When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *ClusterProvisionValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "clusterprovisionvalidator",
	}).Info("Registering validation REST resource")
	// NOTE: This GVR is meant to be different than the ClusterProvision CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    "admission.hive.openshift.io",
			Version:  "v1alpha1",
			Resource: "clusterprovisionvalidators",
		},
		"clusterprovisionvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterProvisionValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    "admission.hive.openshift.io",
		"version":  "v1alpha1",
		"resource": "clusterprovisionvalidator",
	}).Info("Initializing validation REST resource")

	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	a.decoder = serializer.NewCodecFactory(scheme).UniversalDecoder(hivev1.SchemeGroupVersion)

	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *ClusterProvisionValidatingAdmissionHook) Validate(request *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	logger := log.WithFields(log.Fields{
		"operation": request.Operation,
		"group":     request.Resource.Group,
		"version":   request.Resource.Version,
		"resource":  request.Resource.Resource,
		"method":    "Validate",
	})

	if !a.shouldValidate(request, logger) {
		logger.Info("Skipping validation for request")
		// The request object isn't something that this validator should validate.
		// Therefore, we say that it's allowed.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	logger.Info("Validating request")

	switch request.Operation {
	case admissionv1beta1.Create:
		return a.validateCreateRequest(request, logger)
	case admissionv1beta1.Update:
		return a.validateUpdateRequest(request, logger)
	default:
		logger.Info("Successful validation")
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
}

// shouldValidate explicitly checks if the request should validated. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *ClusterProvisionValidatingAdmissionHook) shouldValidate(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) bool {
	logger = logger.WithField("method", "shouldValidate")

	if request.Resource.Group != clusterProvisionGroup {
		logger.Debug("Returning False, not our group")
		return false
	}

	if request.Resource.Version != clusterProvisionVersion {
		logger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if request.Resource.Resource != clusterProvisionResource {
		logger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	logger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreateRequest specifically validates create operations for ClusterProvision objects.
func (a *ClusterProvisionValidatingAdmissionHook) validateCreateRequest(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) *admissionv1beta1.AdmissionResponse {
	logger = logger.WithField("method", "validateCreateRequest")

	newObject, resp := a.decode(&request.Object, logger.WithField("decode", "Object"))
	if resp != nil {
		return resp
	}

	logger = logger.
		WithField("object.Name", newObject.Name).
		WithField("object.Namespace", newObject.Namespace)

	if allErrs := validateClusterProvisionCreate(newObject); len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateUpdateRequest specifically validates update operations for ClusterProvision objects.
func (a *ClusterProvisionValidatingAdmissionHook) validateUpdateRequest(request *admissionv1beta1.AdmissionRequest, logger log.FieldLogger) *admissionv1beta1.AdmissionResponse {
	logger = logger.WithField("method", "validateUpdateRequest")

	newObject, resp := a.decode(&request.Object, logger.WithField("decode", "Object"))
	if resp != nil {
		return resp
	}

	logger = logger.
		WithField("object.Name", newObject.Name).
		WithField("object.Namespace", newObject.Namespace)

	oldObject, resp := a.decode(&request.OldObject, logger.WithField("decode", "OldObject"))
	if resp != nil {
		return resp
	}

	if allErrs := validateClusterProvisionUpdate(oldObject, newObject); len(allErrs) > 0 {
		logger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(request.Kind).GroupKind(), request.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	logger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

func (a *ClusterProvisionValidatingAdmissionHook) decode(raw *runtime.RawExtension, logger log.FieldLogger) (*hivev1.ClusterProvision, *admissionv1beta1.AdmissionResponse) {
	obj := &hivev1.ClusterProvision{}
	if _, _, err := a.decoder.Decode(raw.Raw, nil, obj); err != nil {
		logger.WithError(err).Error("failed to decode")
		return nil, &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: err.Error(),
			},
		}
	}
	return obj, nil
}

func validateClusterProvisionCreate(provision *hivev1.ClusterProvision) field.ErrorList {
	allErrs := field.ErrorList{}
	allErrs = append(allErrs, validateClusterProvisionSpecInvariants(&provision.Spec, field.NewPath("spec"))...)
	return allErrs
}

func validateClusterProvisionUpdate(old, new *hivev1.ClusterProvision) field.ErrorList {
	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")
	allErrs = append(allErrs, validateClusterProvisionSpecInvariants(&new.Spec, specPath)...)
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.ClusterDeployment.Name, old.Spec.ClusterDeployment.Name, specPath.Child("clusterDeployment", "name"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.PodSpec, old.Spec.PodSpec, specPath.Child("podSpec"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.Attempt, old.Spec.Attempt, specPath.Child("attempt"))...)
	if old.Spec.Stage != new.Spec.Stage {
		badStageTransition := true
		switch old.Spec.Stage {
		case hivev1.ClusterProvisionStageInitializing:
			badStageTransition = new.Spec.Stage == hivev1.ClusterProvisionStageComplete
		case hivev1.ClusterProvisionStageProvisioning:
			badStageTransition = new.Spec.Stage == hivev1.ClusterProvisionStageInitializing
		}
		if badStageTransition {
			allErrs = append(allErrs, field.Invalid(specPath.Child("stage"), new.Spec.Stage, fmt.Sprintf("cannot transition from %s to %s", old.Spec.Stage, new.Spec.Stage)))
		}
	}
	if old.Spec.ClusterID != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.ClusterID, old.Spec.ClusterID, specPath.Child("clusterID"))...)
	}
	if old.Spec.InfraID != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.InfraID, old.Spec.InfraID, specPath.Child("infraID"))...)
	}
	if old.Spec.AdminKubeconfigSecret != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.AdminKubeconfigSecret, old.Spec.AdminKubeconfigSecret, specPath.Child("adminKubeconfigSecret"))...)
	}
	if old.Spec.AdminPasswordSecret != nil {
		allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.AdminPasswordSecret, old.Spec.AdminPasswordSecret, specPath.Child("adminPasswordSecret"))...)
	}
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.PrevClusterID, old.Spec.PrevClusterID, specPath.Child("prevClusterID"))...)
	allErrs = append(allErrs, validation.ValidateImmutableField(new.Spec.PrevInfraID, old.Spec.PrevInfraID, specPath.Child("prevInfraID"))...)
	return allErrs
}

func validateClusterProvisionSpecInvariants(spec *hivev1.ClusterProvisionSpec, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if spec.ClusterDeployment.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("clusterDeployment", "name"), "must have reference to clusterdeployment"))
	}
	if spec.Attempt < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("attempt"), spec.Attempt, "attempt number must not be negative"))
	}
	if !validProvisionStages[spec.Stage] {
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("stage"), spec.Stage, validProvisionStageValues))
	}
	if len(spec.PodSpec.Containers) == 0 {
		if spec.Attempt != 0 {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("attempt"), spec.Attempt, "attempt number must not be set for pre-installed cluster"))
		}
		if spec.Stage != hivev1.ClusterProvisionStageComplete {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("stage"), spec.Stage, fmt.Sprintf("stage must be %s for pre-installed cluster", hivev1.ClusterProvisionStageComplete)))
		}
		if spec.PrevClusterID != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("prevClusterID"), spec.PrevClusterID, "previous cluster ID must not be set for pre-installed cluster"))
		}
		if spec.PrevInfraID != nil {
			allErrs = append(allErrs, field.Invalid(fldPath.Child("prevInfraID"), spec.PrevInfraID, "previous infra ID must not be set for pre-installed cluster"))
		}
	}
	if spec.Stage == hivev1.ClusterProvisionStageProvisioning || spec.Stage == hivev1.ClusterProvisionStageComplete {
		if spec.InfraID == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("infraID"), fmt.Sprintf("infra ID must be set for %s or %s cluster", hivev1.ClusterProvisionStageProvisioning, hivev1.ClusterProvisionStageComplete)))
		}
	}
	if spec.Stage == hivev1.ClusterProvisionStageComplete {
		if spec.AdminKubeconfigSecret == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("adminKubeConfigSecret"), fmt.Sprintf("admin kubeconfig secret must be set for %s cluster", hivev1.ClusterProvisionStageComplete)))
		}
		if spec.AdminPasswordSecret == nil {
			allErrs = append(allErrs, field.Required(fldPath.Child("adminPasswordSecret"), fmt.Sprintf("admin password secret must be set for %s cluster", hivev1.ClusterProvisionStageComplete)))
		}
	}
	if spec.ClusterID != nil && *spec.ClusterID == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("clusterID"), spec.ClusterID, "cluster ID must not be an empty string"))
	}
	if spec.InfraID != nil && *spec.InfraID == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("infraID"), spec.InfraID, "infra ID must not be an empty string"))
	}
	if spec.AdminKubeconfigSecret != nil && spec.AdminKubeconfigSecret.Name == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("adminKubeConfigSecret", "name"), spec.AdminKubeconfigSecret.Name, "admin kubeconfig secret must have a non-empty name"))
	}
	if spec.AdminPasswordSecret != nil && spec.AdminPasswordSecret.Name == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("adminPasswordSecret", "name"), spec.AdminPasswordSecret.Name, "admin password secret must have a non-empty name"))
	}
	if spec.PrevClusterID != nil && *spec.PrevClusterID == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("prevClusterID"), spec.PrevClusterID, "previous cluster ID must not be an empty string"))
	}
	if spec.PrevInfraID != nil && *spec.PrevInfraID == "" {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("prevInfraID"), spec.PrevInfraID, "previous infra ID must not be an empty string"))
	}
	return allErrs
}

func schemaGVK(gvk metav1.GroupVersionKind) schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    gvk.Kind,
	}
}

package validatingwebhooks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"

	"github.com/openshift/hive/pkg/manageddns"
)

const (
	clusterDeploymentGroup    = "hive.openshift.io"
	clusterDeploymentVersion  = "v1"
	clusterDeploymentResource = "clusterdeployments"

	clusterDeploymentAdmissionGroup   = "admission.hive.openshift.io"
	clusterDeploymentAdmissionVersion = "v1"
)

var (
	mutableFields = []string{"CertificateBundles", "ClusterMetadata", "ControlPlaneConfig", "Ingress", "Installed", "PreserveOnDelete"}
)

// ClusterDeploymentValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterDeploymentValidatingAdmissionHook struct {
	validManagedDomains []string
}

// NewClusterDeploymentValidatingAdmissionHook constructs a new ClusterDeploymentValidatingAdmissionHook
func NewClusterDeploymentValidatingAdmissionHook() *ClusterDeploymentValidatingAdmissionHook {
	logger := log.WithField("validating_webhook", "clusterdeployment")
	managedDomains, err := manageddns.ReadManagedDomainsFile()
	if err != nil {
		logger.WithError(err).Fatal("Unable to read managedDomains file")
	}
	domains := []string{}
	for _, md := range managedDomains {
		domains = append(domains, md.Domains...)
	}
	logger.WithField("managedDomains", domains).Info("Read managed domains")
	return &ClusterDeploymentValidatingAdmissionHook{
		validManagedDomains: domains,
	}
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1/clusterdeploymentvalidators".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *ClusterDeploymentValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": "clusterdeploymentvalidator",
	}).Info("Registering validation REST resource")

	// NOTE: This GVR is meant to be different than the ClusterDeployment CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    clusterDeploymentAdmissionGroup,
			Version:  clusterDeploymentAdmissionVersion,
			Resource: "clusterdeploymentvalidators",
		},
		"clusterdeploymentvalidator"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterDeploymentValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": "clusterdeploymentvalidator",
	}).Info("Initializing validation REST resource")
	return nil // No initialization needed right now.
}

// Validate is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission validation request.
func (a *ClusterDeploymentValidatingAdmissionHook) Validate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
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
func (a *ClusterDeploymentValidatingAdmissionHook) shouldValidate(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldValidate",
	})

	if admissionSpec.Resource.Group != clusterDeploymentGroup {
		contextLogger.Debug("Returning False, not our group")
		return false
	}

	if admissionSpec.Resource.Version != clusterDeploymentVersion {
		contextLogger.Debug("Returning False, it's our group, but not the right version")
		return false
	}

	if admissionSpec.Resource.Resource != clusterDeploymentResource {
		contextLogger.Debug("Returning False, it's our group and version, but not the right resource")
		return false
	}

	// If we get here, then we're supposed to validate the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// validateCreate specifically validates create operations for ClusterDeployment objects.
func (a *ClusterDeploymentValidatingAdmissionHook) validateCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateCreate",
	})

	newObject := &hivev1.ClusterDeployment{}
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

	// TODO: Put Create Validation Here (or in openAPIV3Schema validation section of crd)

	if len(newObject.Name) > validation.DNS1123LabelMaxLength {
		message := fmt.Sprintf("Invalid cluster deployment name (.meta.name): %s", validation.MaxLenError(validation.DNS1123LabelMaxLength))
		contextLogger.Error(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if len(newObject.Spec.ClusterName) > validation.DNS1123LabelMaxLength {
		message := fmt.Sprintf("Invalid cluster name (.spec.clusterName): %s", validation.MaxLenError(validation.DNS1123LabelMaxLength))
		contextLogger.Error(message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// validate the ingress
	if ingressValidationResult := validateIngress(newObject, contextLogger); ingressValidationResult != nil {
		return ingressValidationResult
	}

	// validate the certificate bundles
	if r := validateCertificateBundles(newObject, contextLogger); r != nil {
		return r
	}

	if newObject.Spec.ManageDNS {
		if !validateDomain(newObject.Spec.BaseDomain, a.validManagedDomains) {
			message := "The base domain must be a child of one of the managed domains for ClusterDeployments with manageDNS set to true"
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
	}

	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	if !newObject.Spec.Installed {
		if newObject.Spec.Provisioning == nil {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning"), "provisioning is required if not installed"))

		} else if newObject.Spec.Provisioning.InstallConfigSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning", "installConfigSecretRef", "name"), "must specify an InstallConfig"))
		}
	}

	platformPath := specPath.Child("platform")
	numberOfPlatforms := 0
	canManageDNS := false
	if newObject.Spec.Platform.AWS != nil {
		numberOfPlatforms++
		canManageDNS = true
		aws := newObject.Spec.Platform.AWS
		awsPath := platformPath.Child("aws")
		if aws.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(awsPath.Child("credentialsSecretRef", "name"), "must specify secrets for AWS access"))
		}
		if aws.Region == "" {
			allErrs = append(allErrs, field.Required(awsPath.Child("region"), "must specify AWS region"))
		}
	}
	if newObject.Spec.Platform.Azure != nil {
		numberOfPlatforms++
		azure := newObject.Spec.Platform.Azure
		azurePath := platformPath.Child("azure")
		if azure.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("credentialsSecretRef", "name"), "must specify secrets for Azure access"))
		}
		if azure.Region == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("region"), "must specify Azure region"))
		}
		if azure.BaseDomainResourceGroupName == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("baseDomainResourceGroupName"), "must specify the Azure resource group for the base domain"))
		}
	}
	if newObject.Spec.Platform.GCP != nil {
		numberOfPlatforms++
		canManageDNS = true
		gcp := newObject.Spec.Platform.GCP
		gcpPath := platformPath.Child("gcp")
		if gcp.CredentialsSecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("credentialsSecretRef", "name"), "must specify secrets for GCP access"))
		}
		if gcp.Region == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("region"), "must specify GCP region"))
		}
	}
	if newObject.Spec.Platform.BareMetal != nil {
		numberOfPlatforms++
	}
	switch {
	case numberOfPlatforms == 0:
		allErrs = append(allErrs, field.Required(platformPath, "must specify a platform"))
	case numberOfPlatforms > 1:
		allErrs = append(allErrs, field.Invalid(platformPath, newObject.Spec.Platform, "must specify only a single platform"))
	}
	if !canManageDNS && newObject.Spec.ManageDNS {
		allErrs = append(allErrs, field.Invalid(specPath.Child("manageDNS"), newObject.Spec.ManageDNS, "cannot manage DNS for the selected platform"))
	}

	if newObject.Spec.Provisioning != nil {
		if newObject.Spec.Provisioning.SSHPrivateKeySecretRef != nil && newObject.Spec.Provisioning.SSHPrivateKeySecretRef.Name == "" {
			allErrs = append(allErrs, field.Required(specPath.Child("provisioning", "sshPrivateKeySecretRef", "name"), "must specify a name for the ssh private key secret if the ssh private key secret is specified"))
		}
	}

	if len(allErrs) > 0 {
		status := errors.NewInvalid(schemaGVK(admissionSpec.Kind).GroupKind(), admissionSpec.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// validateUpdate specifically validates update operations for ClusterDeployment objects.
func (a *ClusterDeploymentValidatingAdmissionHook) validateUpdate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "validateUpdate",
	})

	newObject := &hivev1.ClusterDeployment{}
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

	oldObject := &hivev1.ClusterDeployment{}
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

	// Add the new data to the contextLogger
	contextLogger.Data["oldObject.Name"] = oldObject.Name

	hasChangedImmutableField, changedFieldName := hasChangedImmutableField(&oldObject.Spec, &newObject.Spec)
	if hasChangedImmutableField {
		message := fmt.Sprintf("Attempted to change ClusterDeployment.Spec.%v. ClusterDeployment.Spec is immutable except for %v", changedFieldName, mutableFields)
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// validate the newly incoming ingress
	if ingressValidationResult := validateIngress(newObject, contextLogger); ingressValidationResult != nil {
		return ingressValidationResult
	}

	// Now catch the case where there was a previously defined list and now it's being emptied
	hasClearedOutPreviouslyDefinedIngressList := hasClearedOutPreviouslyDefinedIngressList(&oldObject.Spec, &newObject.Spec)
	if hasClearedOutPreviouslyDefinedIngressList {
		message := fmt.Sprintf("Previously defined a list of ingress objects, must provide a default ingress object")
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	allErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	if newObject.Spec.Installed {
		if newObject.Spec.ClusterMetadata != nil {
			if oldObject.Spec.Installed {
				allErrs = append(allErrs, apivalidation.ValidateImmutableField(newObject.Spec.ClusterMetadata, oldObject.Spec.ClusterMetadata, specPath.Child("clusterMetadata"))...)
			}
		} else {
			allErrs = append(allErrs, field.Required(specPath.Child("clusterMetadata"), "installed cluster must have cluster metadata"))
		}
	} else {
		if oldObject.Spec.Installed {
			allErrs = append(allErrs, field.Invalid(specPath.Child("installed"), newObject.Spec.Installed, "cannot make uninstalled once installed"))
		}
	}

	if len(allErrs) > 0 {
		contextLogger.WithError(allErrs.ToAggregate()).Info("failed validation")
		status := errors.NewInvalid(schemaGVK(admissionSpec.Kind).GroupKind(), admissionSpec.Name, allErrs).Status()
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result:  &status,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful validation")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// isFieldMutable says whether the ClusterDeployment.spec field is meant to be mutable or not.
func isFieldMutable(value string) bool {
	for _, mutableField := range mutableFields {
		if value == mutableField {
			return true
		}
	}

	return false
}

// hasChangedImmutableField determines if a ClusterDeployment.spec immutable field was changed.
func hasChangedImmutableField(oldObject, newObject *hivev1.ClusterDeploymentSpec) (bool, string) {
	ooElem := reflect.ValueOf(oldObject).Elem()
	noElem := reflect.ValueOf(newObject).Elem()

	for i := 0; i < ooElem.NumField(); i++ {
		ooFieldName := ooElem.Type().Field(i).Name
		ooValue := ooElem.Field(i).Interface()
		noValue := noElem.Field(i).Interface()

		if !isFieldMutable(ooFieldName) && !reflect.DeepEqual(ooValue, noValue) {
			// The field isn't mutable -and- has been changed. DO NOT ALLOW.
			return true, ooFieldName
		}
	}

	return false, ""
}

func hasClearedOutPreviouslyDefinedIngressList(oldObject, newObject *hivev1.ClusterDeploymentSpec) bool {
	// We don't allow a ClusterDeployment which had previously defined a list of Ingress objects
	// to then be cleared out. It either must be cleared from the beginning (ie just use default behavior),
	// or the ClusterDeployment must continue to define at least the 'default' ingress object.
	if len(oldObject.Ingress) > 0 && len(newObject.Ingress) == 0 {
		return true
	}

	return false
}

func validateIngressDomainsShareClusterDomain(newObject *hivev1.ClusterDeploymentSpec) bool {
	// ingress entries must share the same domain as the cluster
	// so watch for an ingress domain ending in: .<clusterName>.<baseDomain>
	regexString := fmt.Sprintf(`(?i).*\.%s.%s$`, newObject.ClusterName, newObject.BaseDomain)
	sharedSubdomain := regexp.MustCompile(regexString)

	for _, ingress := range newObject.Ingress {
		if !sharedSubdomain.Match([]byte(ingress.Domain)) {
			return false
		}
	}
	return true
}
func validateIngressDomainsNotWildcard(newObject *hivev1.ClusterDeploymentSpec) bool {
	// check for domains with leading '*'
	// the * is unnecessary as the ingress controller assumes a wildcard
	for _, ingress := range newObject.Ingress {
		if ingress.Domain[0] == '*' {
			return false
		}
	}
	return true
}

func validateIngressServingCertificateExists(newObject *hivev1.ClusterDeploymentSpec) bool {
	// Include the empty string in the set of certs so that an ingress with
	// an empty serving certificate passes.
	certs := sets.NewString("")
	for _, cert := range newObject.CertificateBundles {
		certs.Insert(cert.Name)
	}
	for _, ingress := range newObject.Ingress {
		if !certs.Has(ingress.ServingCertificate) {
			return false
		}
	}
	return true
}

// empty ingress is allowed (for create), but if it's non-zero
// it must include an entry for 'default'
func validateIngressList(newObject *hivev1.ClusterDeploymentSpec) bool {
	if len(newObject.Ingress) == 0 {
		return true
	}

	defaultFound := false
	for _, ingress := range newObject.Ingress {
		if ingress.Name == "default" {
			defaultFound = true
		}
	}
	if !defaultFound {
		return false
	}

	return true
}

func validateDomain(domain string, validDomains []string) bool {
	for _, validDomain := range validDomains {
		if strings.HasSuffix(domain, "."+validDomain) {
			childPart := strings.TrimSuffix(domain, "."+validDomain)
			if !strings.Contains(childPart, ".") {
				return true
			}
		}
	}
	return false
}

func validateIngress(newObject *hivev1.ClusterDeployment, contextLogger *log.Entry) *admissionv1beta1.AdmissionResponse {
	if !validateIngressList(&newObject.Spec) {
		message := fmt.Sprintf("Ingress list must include a default entry")
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressDomainsNotWildcard(&newObject.Spec) {
		message := "Ingress domains must not lead with *"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressDomainsShareClusterDomain(&newObject.Spec) {
		message := "Ingress domains must share the same domain as the cluster"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if !validateIngressServingCertificateExists(&newObject.Spec) {
		message := "Ingress has serving certificate that does not exist in certificate bundle"
		contextLogger.Infof("Failed validation: %v", message)
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// everything passed
	return nil
}

func validateCertificateBundles(newObject *hivev1.ClusterDeployment, contextLogger *log.Entry) *admissionv1beta1.AdmissionResponse {
	for _, certBundle := range newObject.Spec.CertificateBundles {
		if certBundle.Name == "" {
			message := "Certificate bundle is missing a name"
			contextLogger.Infof("Failed validation: %v", message)
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
		if certBundle.CertificateSecretRef.Name == "" {
			message := "Certificate bundle is missing a secret reference"
			contextLogger.Infof("Failed validation: %v", message)
			return &admissionv1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
					Message: message,
				},
			}
		}
	}
	return nil
}

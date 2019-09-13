package validatingwebhooks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/rest"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	clusterDeploymentGroup    = "hive.openshift.io"
	clusterDeploymentVersion  = "v1alpha1"
	clusterDeploymentResource = "clusterdeployments"

	clusterDeploymentAdmissionGroup    = "admission.hive.openshift.io"
	clusterDeploymentAdmissionVersion  = "v1alpha1"
	clusterDeploymentAdmissionResource = "clusterdeployments"

	// ManagedDomainsFileEnvVar if present, points to a simple text
	// file that includes a valid managed domain per line. Cluster deployments
	// requesting that their domains be managed must have a base domain
	// that is a direct child of one of the valid domains.
	ManagedDomainsFileEnvVar = "MANAGED_DOMAINS_FILE"
)

var (
	mutableFields = []string{"CertificateBundles", "Compute", "ControlPlaneConfig", "Ingress", "Installed", "PreserveOnDelete"}
)

// ClusterDeploymentValidatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterDeploymentValidatingAdmissionHook struct {
	validManagedDomains []string
}

// NewClusterDeploymentValidatingAdmissionHook constructs a new ClusterDeploymentValidatingAdmissionHook
func NewClusterDeploymentValidatingAdmissionHook() *ClusterDeploymentValidatingAdmissionHook {
	managedDomainsFile := os.Getenv(ManagedDomainsFileEnvVar)
	logger := log.WithField("validating_webhook", "clusterdeployment")
	webhook := &ClusterDeploymentValidatingAdmissionHook{}
	if len(managedDomainsFile) == 0 {
		logger.Debug("No managed domains file specified")
		return webhook
	}
	logger.WithField("file", managedDomainsFile).Debug("Managed domains file specified")
	var err error
	webhook.validManagedDomains, err = readManagedDomainsFile(managedDomainsFile)
	if err != nil {
		logger.WithError(err).WithField("file", managedDomainsFile).Fatal("Unable to read managedDomains file")
	}

	return webhook
}

// ValidatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1alpha1/clusterdeployments".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Validate() method below.
func (a *ClusterDeploymentValidatingAdmissionHook) ValidatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": clusterDeploymentAdmissionResource,
	}).Info("Registering validation REST resource")

	// NOTE: This GVR is meant to be different than the ClusterDeployment CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    clusterDeploymentAdmissionGroup,
			Version:  clusterDeploymentAdmissionVersion,
			Resource: clusterDeploymentAdmissionResource,
		},
		"clusterdeployment"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterDeploymentValidatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": clusterDeploymentAdmissionResource,
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

	if newObject.Spec.SSHKey.Name == "" {
		allErrs = append(allErrs, field.Required(specPath.Child("sshKey", "name"), "must specify an SSH key to use"))
	}

	platformPath := specPath.Child("platform")
	platformSecretsPath := specPath.Child("platformSecrets")
	numberOfPlatforms := 0
	canManageDNS := false
	if newObject.Spec.Platform.AWS != nil {
		numberOfPlatforms++
		canManageDNS = true
		if newObject.Spec.PlatformSecrets.AWS == nil {
			allErrs = append(allErrs, field.Required(platformSecretsPath.Child("aws"), "must specify secrets for AWS access"))
		}
		aws := newObject.Spec.Platform.AWS
		awsPath := platformPath.Child("aws")
		if aws.Region == "" {
			allErrs = append(allErrs, field.Required(awsPath.Child("region"), "must specify AWS region"))
		}
	}
	if newObject.Spec.Platform.Azure != nil {
		numberOfPlatforms++
		if newObject.Spec.PlatformSecrets.Azure == nil {
			allErrs = append(allErrs, field.Required(platformSecretsPath.Child("azure"), "must specify secrets for Azure access"))
		}
		azure := newObject.Spec.Platform.Azure
		azurePath := platformPath.Child("azure")
		if azure.Region == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("region"), "must specify Azure region"))
		}
		if azure.BaseDomainResourceGroupName == "" {
			allErrs = append(allErrs, field.Required(azurePath.Child("baseDomainResourceGroupName"), "must specify the Azure resource group for the base domain"))
		}
	}
	if newObject.Spec.Platform.GCP != nil {
		numberOfPlatforms++
		if newObject.Spec.PlatformSecrets.GCP == nil {
			allErrs = append(allErrs, field.Required(platformSecretsPath.Child("gcp"), "must specify secrets for GCP access"))
		}
		gcp := newObject.Spec.Platform.GCP
		gcpPath := platformPath.Child("gcp")
		if gcp.ProjectID == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("projectID"), "must specify GCP project ID"))
		}
		if gcp.Region == "" {
			allErrs = append(allErrs, field.Required(gcpPath.Child("region"), "must specify GCP region"))
		}
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

	// Check to make sure only allowed fields under controlPlaneConfig are being modified
	if hasChangedImmutableControlPlaneConfigFields(&oldObject.Spec, &newObject.Spec) {
		message := fmt.Sprintf("Attempt to modify immutable field in controlPlaneConfig (only servingCertificates is mutable)")
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	// Check that existing machinepools aren't modifying the labels and/or taints fields
	hasChangedImmutableMachinePoolFields, computePoolName := hasChangedImmutableMachinePoolFields(&oldObject.Spec, &newObject.Spec)
	if hasChangedImmutableMachinePoolFields {
		message := fmt.Sprintf("Detected attempt to change Labels or Taints on existing Compute object: %s", computePoolName)
		contextLogger.Infof("Failed validation: %v", message)

		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
	}

	if oldObject.Spec.Installed && !newObject.Spec.Installed {
		allErrs := field.ErrorList{}
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec", "installed"), newObject.Spec.Installed, "cannot make uninstalled once installed"))
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

// currently only allow controlPlaneConfig.servingCertificates to be mutable
func hasChangedImmutableControlPlaneConfigFields(origObject, newObject *hivev1.ClusterDeploymentSpec) bool {
	origCopy := origObject.ControlPlaneConfig.DeepCopy()
	newCopy := newObject.ControlPlaneConfig.DeepCopy()

	// blank out the servingCertificates, since we don't care if they're different
	origCopy.ServingCertificates = hivev1.ControlPlaneServingCertificateSpec{}
	newCopy.ServingCertificates = hivev1.ControlPlaneServingCertificateSpec{}

	if !reflect.DeepEqual(origCopy, newCopy) {
		return true
	}

	return false
}

func hasChangedImmutableMachinePoolFields(oldObject, newObject *hivev1.ClusterDeploymentSpec) (bool, string) {
	// any pre-existing compute machinepool should not mutate the Labels or Taints fields

	for _, newMP := range newObject.Compute {
		origMP := getOriginalMachinePool(oldObject.Compute, newMP.Name)
		if origMP == nil {
			// no mutate checks needed for new compute machinepool
			continue
		}

		// Check if labels are being changed
		if !reflect.DeepEqual(origMP.Labels, newMP.Labels) {
			return true, newMP.Name
		}

		// Check if taints are being changed
		if !reflect.DeepEqual(origMP.Taints, newMP.Taints) {
			return true, newMP.Name
		}
	}

	return false, ""
}

func getOriginalMachinePool(origMachinePools []hivev1.MachinePool, name string) *hivev1.MachinePool {
	var origMP *hivev1.MachinePool

	for i, mp := range origMachinePools {
		if mp.Name == name {
			origMP = &origMachinePools[i]
			break
		}
	}
	return origMP
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

func readManagedDomainsFile(fileName string) ([]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		s := scanner.Text()
		s = strings.TrimSpace(s)
		if len(s) > 0 {
			result = append(result, s)
		}
	}
	return result, nil
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

	// everything passed
	return nil
}

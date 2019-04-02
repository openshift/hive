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
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	mutableFields = []string{"Compute", "Ingress", "PreserveOnDelete"}
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

	if newObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["object.Name"] = newObject.Name
	}

	// TODO: Put Create Validation Here (or in openAPIV3Schema validation section of crd)

	if !validateIngressList(&newObject.Spec) {
		message := "If defining the Ingress list, it must include a default ingress entry"
		return &admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status: metav1.StatusFailure, Code: http.StatusBadRequest, Reason: metav1.StatusReasonBadRequest,
				Message: message,
			},
		}
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

	if newObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["object.Name"] = newObject.Name
	}

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

	if oldObject != nil {
		// Add the new data to the contextLogger
		contextLogger.Data["oldObject.Name"] = oldObject.Name
	}

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

	// First check whether the newlist is valid at all
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
	// to then be cleared out. It either must be cleared from the begining (ie just use default behavior),
	// or the ClusterDeployment must continue to define at least the 'default' ingress object.
	if len(oldObject.Ingress) > 0 && len(newObject.Ingress) == 0 {
		return true
	}

	return false
}

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

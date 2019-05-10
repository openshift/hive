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

package mutatingwebhooks

import (
	"encoding/json"
	"net/http"

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
	clusterDeploymentAdmissionResource = "mutateclusterdeployments"

	setPresentStatePatchString = `[{"op":"add", "path":"/spec/state", "value":"Present"}]`
)

var setPresentStatePatch = []byte(setPresentStatePatchString)

// ClusterDeploymentMutatingAdmissionHook is a struct that is used to reference what code should be run by the generic-admission-server.
type ClusterDeploymentMutatingAdmissionHook struct {
}

// MutatingResource is called by generic-admission-server on startup to register the returned REST resource through which the
//                    webhook is accessed by the kube apiserver.
// For example, generic-admission-server uses the data below to register the webhook on the REST resource "/apis/admission.hive.openshift.io/v1alpha1/clusterdeployments".
//              When the kube apiserver calls this registered REST resource, the generic-admission-server calls the Admit() method below.
func (a *ClusterDeploymentMutatingAdmissionHook) MutatingResource() (plural schema.GroupVersionResource, singular string) {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": clusterDeploymentAdmissionResource,
	}).Info("Registering mutating REST resource")

	// NOTE: This GVR is meant to be different than the ClusterDeployment CRD GVR which has group "hive.openshift.io".
	return schema.GroupVersionResource{
			Group:    clusterDeploymentAdmissionGroup,
			Version:  clusterDeploymentAdmissionVersion,
			Resource: clusterDeploymentAdmissionResource,
		},
		"clusterdeployment"
}

// Initialize is called by generic-admission-server on startup to setup any special initialization that your webhook needs.
func (a *ClusterDeploymentMutatingAdmissionHook) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"group":    clusterDeploymentAdmissionGroup,
		"version":  clusterDeploymentAdmissionVersion,
		"resource": clusterDeploymentAdmissionResource,
	}).Info("Initializing mutating REST resource")
	return nil // No initialization needed right now.
}

// Admit is called by generic-admission-server when the registered REST resource above is called with an admission request.
// Usually it's the kube apiserver that is making the admission request.
func (a *ClusterDeploymentMutatingAdmissionHook) Admit(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "Admit",
	})

	if !a.shouldAdmit(admissionSpec) {
		contextLogger.Info("Skipping admission request")
		// The request object isn't something that this webhook should process.
		// Therefore, we say that it's Allowed.
		return &admissionv1beta1.AdmissionResponse{
			Allowed: true,
		}
	}

	if admissionSpec.Operation == admissionv1beta1.Create {
		contextLogger.Info("Mutating request")
		return a.admitCreate(admissionSpec)
	}

	// We're only mutating creates at this time, so all other operations are explicitly allowed.
	contextLogger.Info("Successful admission")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

// shouldAdmit explicitly checks if the request should processed. For example, this webhook may have accidentally been registered to check
// the validity of some other type of object with a different GVR.
func (a *ClusterDeploymentMutatingAdmissionHook) shouldAdmit(admissionSpec *admissionv1beta1.AdmissionRequest) bool {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "shouldAdmit",
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

	// If we get here, then we're supposed to admit the object.
	contextLogger.Debug("Returning True, passed all prerequisites.")
	return true
}

// admitCreate specifically processes create operations for ClusterDeployment objects.
func (a *ClusterDeploymentMutatingAdmissionHook) admitCreate(admissionSpec *admissionv1beta1.AdmissionRequest) *admissionv1beta1.AdmissionResponse {
	contextLogger := log.WithFields(log.Fields{
		"operation": admissionSpec.Operation,
		"group":     admissionSpec.Resource.Group,
		"version":   admissionSpec.Resource.Version,
		"resource":  admissionSpec.Resource.Resource,
		"method":    "admitCreate",
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

	if newObject.Spec.State == "" {
		patchType := admissionv1beta1.PatchTypeJSONPatch
		contextLogger.Info("Setting default state")
		return &admissionv1beta1.AdmissionResponse{
			Allowed:   true,
			Patch:     setPresentStatePatch,
			PatchType: &patchType,
		}
	}

	// If we get here, then all checks passed, so the object is valid.
	contextLogger.Info("Successful admission")
	return &admissionv1beta1.AdmissionResponse{
		Allowed: true,
	}
}

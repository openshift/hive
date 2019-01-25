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
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/stretchr/testify/assert"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
)

var (
	validClusterDeployment = &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "SameClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: "SameMachinePoolName",
				},
			},
		},
	}

	// Meant to be used to compare new and old as the same values.
	validClusterDeploymentSameValues = &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "SameClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: "SameMachinePoolName",
				},
			},
		},
	}

	validClusterDeploymentDifferentImmutableValue = &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "DifferentClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: "SameMachinePoolName",
				},
			},
		},
	}

	validClusterDeploymentDifferentMutableValue = &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "SameClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: "DifferentMachinePoolName",
				},
			},
		},
	}
)

func TestClusterDeploymentValidatingResource(t *testing.T) {
	// Arrange
	data := ClusterDeploymentValidatingAdmissionHook{}
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1alpha1",
		Resource: "clusterdeployments",
	}
	expectedSingular := "clusterdeployment"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestClusterDeploymentInitialize(t *testing.T) {
	// Arrange
	data := ClusterDeploymentValidatingAdmissionHook{}

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestClusterDeploymentValidate(t *testing.T) {
	cases := []struct {
		name            string
		newObject       *hivev1.ClusterDeployment
		newObjectRaw    []byte
		oldObject       *hivev1.ClusterDeployment
		oldObjectRaw    []byte
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		gvr             *metav1.GroupVersionResource
	}{
		{
			name:            "Test Create Operation is allowed even with mismatch objects",
			oldObject:       validClusterDeployment,
			newObject:       nil,
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test Delete Operation is allowed even with mismatch objects",
			oldObject:       validClusterDeployment,
			newObject:       validClusterDeploymentDifferentImmutableValue,
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is allowed with same data",
			oldObject:       validClusterDeployment,
			newObject:       validClusterDeploymentSameValues,
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is allowed with different mutable data",
			oldObject:       validClusterDeployment,
			newObject:       validClusterDeploymentDifferentMutableValue,
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is NOT allowed with different immutable data",
			oldObject:       validClusterDeployment,
			newObject:       validClusterDeploymentDifferentImmutableValue,
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal new object during create",
			newObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal new object during update",
			newObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal old object during update",
			oldObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "Test doesn't validate with right version and resouce, but wrong group",
			gvr: &metav1.GroupVersionResource{
				Group:    "not the right group",
				Version:  "v1alpha1",
				Resource: "clusterdeployments",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and resource, wrong version",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "not the right version",
				Resource: "clusterdeployments",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and version, wrong resouce",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1alpha1",
				Resource: "not the right resource",
			},
			expectedAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := ClusterDeploymentValidatingAdmissionHook{}

			if tc.gvr == nil {
				tc.gvr = &metav1.GroupVersionResource{
					Group:    "hive.openshift.io",
					Version:  "v1alpha1",
					Resource: "clusterdeployments",
				}
			}

			if tc.newObjectRaw == nil {
				tc.newObjectRaw, _ = json.Marshal(tc.newObject)
			}

			if tc.oldObjectRaw == nil {
				tc.oldObjectRaw, _ = json.Marshal(tc.oldObject)
			}

			request := &admissionv1beta1.AdmissionRequest{
				Operation: tc.operation,
				Resource:  *tc.gvr,
				Object: runtime.RawExtension{
					Raw: tc.newObjectRaw,
				},
				OldObject: runtime.RawExtension{
					Raw: tc.oldObjectRaw,
				},
			}

			// Act
			response := data.Validate(request)

			// Assert
			assert.Equal(t, tc.expectedAllowed, response.Allowed)
		})
	}
}

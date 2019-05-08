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

func TestSyncSetValidatingResource(t *testing.T) {
	// Arrange
	data := SyncSetValidatingAdmissionHook{}
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1alpha1",
		Resource: "syncsets",
	}
	expectedSingular := "syncset"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestSyncSetInitialize(t *testing.T) {
	// Arrange
	data := SyncSetValidatingAdmissionHook{}

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestSyncSetValidate(t *testing.T) {
	cases := []struct {
		name            string
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		syncSet         *hivev1.SyncSet
	}{
		{
			name:            "Test valid patch type create",
			operation:       admissionv1beta1.Create,
			syncSet:         testValidPatchSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid patch type create",
			operation:       admissionv1beta1.Create,
			syncSet:         testInvalidPatchSyncSet(),
			expectedAllowed: false,
		},
		{
			name:            "Test valid patch type update",
			operation:       admissionv1beta1.Update,
			syncSet:         testValidPatchSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid patch type update",
			operation:       admissionv1beta1.Update,
			syncSet:         testInvalidPatchSyncSet(),
			expectedAllowed: false,
		},
		{
			name:            "Test create with no patches",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test update with no patches",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSet(),
			expectedAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := SyncSetValidatingAdmissionHook{}

			objectRaw, _ := json.Marshal(tc.syncSet)

			gvr := metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1alpha1",
				Resource: "syncsets",
			}

			request := &admissionv1beta1.AdmissionRequest{
				Operation: tc.operation,
				Resource:  gvr,
				Object: runtime.RawExtension{
					Raw: objectRaw,
				},
				OldObject: runtime.RawExtension{
					Raw: objectRaw,
				},
			}

			response := data.Validate(request)
			assert.Equal(t, tc.expectedAllowed, response.Allowed)
		})
	}
}

func testValidPatchSyncSet() *hivev1.SyncSet {
	return testPatchSyncSet("merge")
}

func testInvalidPatchSyncSet() *hivev1.SyncSet {
	return testPatchSyncSet("application/json-patch+json")
}

func testPatchSyncSet(patchType string) *hivev1.SyncSet {
	ss := testSyncSet()
	ss.Spec = hivev1.SyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Patches: []hivev1.SyncObjectPatch{
				{
					Patch:     "blah",
					PatchType: "json",
				},
				{
					Patch:     "foo",
					PatchType: patchType,
				},
			},
		},
	}
	return ss
}

func testSyncSet() *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sync-set",
			Namespace: "test-namespace",
		},
	}
}

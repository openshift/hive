package v1

import (
	"encoding/json"
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/stretchr/testify/assert"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClusterImageSetValidatingResource(t *testing.T) {
	// Arrange
	data := NewClusterImageSetValidatingAdmissionHook(*createDecoder())
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1",
		Resource: "clusterimagesetvalidators",
	}
	expectedSingular := "clusterimagesetvalidator"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestClusterImageSetInitialize(t *testing.T) {
	// Arrange
	data := NewClusterImageSetValidatingAdmissionHook(*createDecoder())

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestClusterImageSetValidate(t *testing.T) {
	cases := []struct {
		name            string
		newSpec         hivev1.ClusterImageSetSpec
		oldSpec         hivev1.ClusterImageSetSpec
		newObjectRaw    []byte
		oldObjectRaw    []byte
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		gvr             *metav1.GroupVersionResource
	}{
		{
			name: "Test valid ClusterImageSet.Spec with release image",
			newSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "image:tag",
			},
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Test valid ClusterImageSet.Spec with digest",
			newSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "image@sha256:abc",
			},
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Test invalid ClusterImageSet.Spec with digest",
			newSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "image@SHA256:abc",
			},
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Test invalid ClusterImageSet.Spec with digest",
			newSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "image@sh256:abc##",
			},
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test empty ClusterImageSet.Spec value",
			newSpec:         hivev1.ClusterImageSetSpec{},
			operation:       admissionv1beta1.Create,
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
			name: "Test ClusterImageSet.Spec is mutable (updates allowed)",
			newSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "a:tag",
			},
			oldSpec: hivev1.ClusterImageSetSpec{
				ReleaseImage: "b:tag",
			},
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test that we don't validate deletes",
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right version and resource, but wrong group",
			gvr: &metav1.GroupVersionResource{
				Group:    "not the right group",
				Version:  "v1",
				Resource: "clusterimagesets",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and resource, wrong version",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "not the right version",
				Resource: "clusterimagesets",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and version, wrong resource",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1",
				Resource: "not the right resource",
			},
			expectedAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := NewClusterImageSetValidatingAdmissionHook(*createDecoder())
			newObject := &hivev1.ClusterImageSet{
				Spec: tc.newSpec,
			}
			oldObject := &hivev1.ClusterImageSet{
				Spec: tc.oldSpec,
			}

			if tc.newObjectRaw == nil {
				tc.newObjectRaw, _ = json.Marshal(newObject)
			}

			if tc.oldObjectRaw == nil {
				tc.oldObjectRaw, _ = json.Marshal(oldObject)
			}

			if tc.gvr == nil {
				tc.gvr = &metav1.GroupVersionResource{
					Group:    "hive.openshift.io",
					Version:  "v1",
					Resource: "clusterimagesets",
				}
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

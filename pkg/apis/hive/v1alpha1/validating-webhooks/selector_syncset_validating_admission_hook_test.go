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

func TestSelectorSyncSetValidatingResource(t *testing.T) {
	// Arrange
	data := SelectorSyncSetValidatingAdmissionHook{}
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1alpha1",
		Resource: "selectorsyncsets",
	}
	expectedSingular := "selectorsyncset"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestSelectorSyncSetInitialize(t *testing.T) {
	// Arrange
	data := SelectorSyncSetValidatingAdmissionHook{}

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestSelectorSyncSetValidate(t *testing.T) {
	cases := []struct {
		name            string
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		selectorSyncSet *hivev1.SelectorSyncSet
	}{
		{
			name:            "Test valid patch type create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testValidPatchSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid patch type create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testInvalidPatchSelectorSyncSet(),
			expectedAllowed: false,
		},
		{
			name:            "Test valid patch type update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testValidPatchSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid patch type update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testInvalidPatchSelectorSyncSet(),
			expectedAllowed: false,
		},
		{
			name:            "Test create syncset with no patches",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test update syncset with no patches",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSet(),
			expectedAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := SelectorSyncSetValidatingAdmissionHook{}

			objectRaw, _ := json.Marshal(tc.selectorSyncSet)

			gvr := metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1alpha1",
				Resource: "selectorsyncsets",
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

func testValidPatchSelectorSyncSet() *hivev1.SelectorSyncSet {
	return testPatchSelectorSyncSet("merge")
}

func testInvalidPatchSelectorSyncSet() *hivev1.SelectorSyncSet {
	return testPatchSelectorSyncSet("application/json-patch+json")
}

func testPatchSelectorSyncSet(patchType string) *hivev1.SelectorSyncSet {
	ss := testSelectorSyncSet()
	ss.Spec = hivev1.SelectorSyncSetSpec{
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

func testSelectorSyncSet() *hivev1.SelectorSyncSet {
	return &hivev1.SelectorSyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-selector-sync-set",
			Namespace: "test-namespace",
		},
	}
}

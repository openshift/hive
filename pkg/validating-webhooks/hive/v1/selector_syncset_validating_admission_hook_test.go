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

func TestSelectorSyncSetValidatingResource(t *testing.T) {
	// Arrange
	data := NewSelectorSyncSetValidatingAdmissionHook(*createDecoder())
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1",
		Resource: "selectorsyncsetvalidators",
	}
	expectedSingular := "selectorsyncsetvalidator"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestSelectorSyncSetInitialize(t *testing.T) {
	// Arrange
	data := NewSelectorSyncSetValidatingAdmissionHook(*createDecoder())

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
		{
			name:            "Test valid SecretReference create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSecretReferenceSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SecretReference update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSecretReferenceSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SecretReference create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSecretReferenceSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SecretReference update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSecretReferenceSelectorSyncSet(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid SecretReference no source name create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSecretReferenceSelectorSyncSet()
				ss.Spec.Secrets[0].SourceRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no target name create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSecretReferenceSelectorSyncSet()
				ss.Spec.Secrets[0].TargetRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no source name update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSecretReferenceSelectorSyncSet()
				ss.Spec.Secrets[0].SourceRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no target name update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSecretReferenceSelectorSyncSet()
				ss.Spec.Secrets[0].TargetRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test valid empty string resourceApplyMode create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = ""
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid empty string resourceApplyMode update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = ""
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Upsert resourceApplyMode create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "Upsert"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Upsert resourceApplyMode update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "Upsert"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Sync resourceApplyMode create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "Sync"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Sync resourceApplyMode update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "Sync"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid resourceApplyMode create",
			operation: admissionv1beta1.Create,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "sync"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid resourceApplyMode update",
			operation: admissionv1beta1.Update,
			selectorSyncSet: func() *hivev1.SelectorSyncSet {
				ss := testSelectorSyncSet()
				ss.Spec.ResourceApplyMode = "sync"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid unmarshalable TypeMeta Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": 798786}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid Role authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "Role"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid Role authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "Role"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid Role authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "Role"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid Role authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "Role"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid RoleBinding authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid RoleBinding authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid RoleBinding authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid RoleBinding authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid ClusterRole authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid ClusterRole authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid ClusterRole authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid ClusterRole authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid ClusterRoleBinding authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid ClusterRoleBinding authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid ClusterRoleBinding authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid ClusterRoleBinding authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: false,
		},

		{
			name:            "Test valid SubjectAccessReview authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SubjectAccessReview authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid SubjectAccessReview authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid SubjectAccessReview authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			selectorSyncSet: testSelectorSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := NewSelectorSyncSetValidatingAdmissionHook(*createDecoder())

			objectRaw, _ := json.Marshal(tc.selectorSyncSet)

			gvr := metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1",
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

func testSecretReferenceSelectorSyncSet() *hivev1.SelectorSyncSet {
	ss := testSelectorSyncSet()
	ss.Spec = hivev1.SelectorSyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Secrets: []hivev1.SecretMapping{
				{
					SourceRef: hivev1.SecretReference{
						Name:      "foo",
						Namespace: "foo",
					},
					TargetRef: hivev1.SecretReference{
						Name:      "foo",
						Namespace: "foo",
					},
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

func testSelectorSyncSetWithResources(resources ...string) *hivev1.SelectorSyncSet {
	ss := testSelectorSyncSet()
	for _, resource := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{Raw: []byte(resource)})
	}
	return ss
}

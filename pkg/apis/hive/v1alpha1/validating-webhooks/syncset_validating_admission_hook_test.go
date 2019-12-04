package validatingwebhooks

import (
	"encoding/json"
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/stretchr/testify/assert"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSyncSetValidatingResource(t *testing.T) {
	// Arrange
	data := SyncSetValidatingAdmissionHook{}
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1alpha1",
		Resource: "syncsetvalidators",
	}
	expectedSingular := "syncsetvalidator"

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
		{
			name:            "Test valid SecretReference create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSecretReferenceSyncSet(),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SecretReference update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSecretReferenceSyncSet(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid SecretReference source kind create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Kind = "Secret"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid SecretReference target kind create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Kind = "Secret"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid SecretReference source kind update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Kind = "Secret"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid SecretReference target kind update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Kind = "Secret"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid SecretReference source kind create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Kind = "Wrong"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target kind create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Kind = "Wrong"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source kind update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Kind = "Wrong"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target kind update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Kind = "Wrong"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no source name create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no target name create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no source name update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no target name update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source fieldPath set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.FieldPath = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source UID set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.UID = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target fieldPath set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.FieldPath = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target UID set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.UID = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source fieldPath set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.FieldPath = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source UID set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.UID = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target fieldPath set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.FieldPath = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference target UID set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Target.UID = "dontset"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test valid SecretReference source apiVersion group set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.APIVersion = "v1"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid SecretReference source apiVersion group set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.APIVersion = "v1"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid SecretReference source apiVersion group set create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.APIVersion = "wrong/v1"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source apiVersion group set update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.SecretReferences[0].Source.APIVersion = "wrong/v1"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid unmarshalable TypeMeta Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": 798786}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid Role authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "Role"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid Role authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "Role"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid Role authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "Role"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid Role authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "Role"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid RoleBinding authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid RoleBinding authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid RoleBinding authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid RoleBinding authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "RoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid ClusterRole authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid ClusterRole authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid ClusterRole authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid ClusterRole authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRole"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test valid ClusterRoleBinding authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid ClusterRoleBinding authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid ClusterRoleBinding authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid ClusterRoleBinding authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "ClusterRoleBinding"}`),
			expectedAllowed: false,
		},

		{
			name:            "Test valid SubjectAccessReview authorization.k8s.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test valid SubjectAccessReview authorization.k8s.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.k8s.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: true,
		},
		{
			name:            "Test invalid SubjectAccessReview authorization.openshift.io Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid SubjectAccessReview authorization.openshift.io Resource update",
			operation:       admissionv1beta1.Update,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "authorization.openshift.io/v1", "kind": "SubjectAccessReview"}`),
			expectedAllowed: false,
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

func testSecretReferenceSyncSet() *hivev1.SyncSet {
	ss := testSyncSet()
	ss.Spec = hivev1.SyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			SecretReferences: []hivev1.SecretReference{
				{
					Source: corev1.ObjectReference{
						Name:      "foo",
						Namespace: "foo",
					},
					Target: corev1.ObjectReference{
						Name:      "foo",
						Namespace: "foo",
					},
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

func testSyncSetWithResources(resources ...string) *hivev1.SyncSet {
	ss := testSyncSet()
	for _, resource := range resources {
		ss.Spec.Resources = append(ss.Spec.Resources, runtime.RawExtension{Raw: []byte(resource)})
	}
	return ss
}

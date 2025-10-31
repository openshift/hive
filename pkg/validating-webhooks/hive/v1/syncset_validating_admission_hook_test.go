package v1

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	machinev1 "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	syncSetNS = "test-namespace"
)

func TestSyncSetValidatingResource(t *testing.T) {
	// Arrange
	data := NewSyncSetValidatingAdmissionHook(*createDecoder())
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1",
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
	data := NewSyncSetValidatingAdmissionHook(*createDecoder())

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
			name:            "Test empty patch type create",
			operation:       admissionv1beta1.Create,
			syncSet:         testPatchSyncSet(""),
			expectedAllowed: true,
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
			name:            "Test empty patch type update",
			operation:       admissionv1beta1.Update,
			syncSet:         testPatchSyncSet(""),
			expectedAllowed: true,
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
			name:      "Test invalid SecretReference no source name create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].SourceRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference source not in SyncSet namespace",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].SourceRef.Namespace = "anotherns"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test valid SecretReference source has empty namespace",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].SourceRef.Namespace = ""
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid SecretReference no target name create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].TargetRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no source name update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].SourceRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid SecretReference no target name update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSecretReferenceSyncSet()
				ss.Spec.Secrets[0].TargetRef.Name = ""
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test valid empty string resourceApplyMode create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = ""
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid empty string resourceApplyMode update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = ""
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Upsert resourceApplyMode create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "Upsert"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Upsert resourceApplyMode update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "Upsert"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Sync resourceApplyMode create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "Sync"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test valid Sync resourceApplyMode update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "Sync"
				return ss
			}(),
			expectedAllowed: true,
		},
		{
			name:      "Test invalid resourceApplyMode create",
			operation: admissionv1beta1.Create,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "sync"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:      "Test invalid resourceApplyMode update",
			operation: admissionv1beta1.Update,
			syncSet: func() *hivev1.SyncSet {
				ss := testSyncSet()
				ss.Spec.ResourceApplyMode = "sync"
				return ss
			}(),
			expectedAllowed: false,
		},
		{
			name:            "Test invalid unmarshalable Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": 798786}`),
			expectedAllowed: false,
		},
		{
			name:            "Test missing Kind Resource create",
			operation:       admissionv1beta1.Create,
			syncSet:         testSyncSetWithResources(`{"apiVersion": "v1"}`),
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
			data := NewSyncSetValidatingAdmissionHook(*createDecoder())

			objectRaw, _ := json.Marshal(tc.syncSet)

			gvr := metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1",
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
			Secrets: []hivev1.SecretMapping{
				{
					SourceRef: hivev1.SecretReference{
						Name:      "foo",
						Namespace: syncSetNS,
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

func testSyncSet() *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sync-set",
			Namespace: syncSetNS,
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

func TestValidateNutanixMachinePoolPlatformInvariants(t *testing.T) {
	tests := []struct {
		name     string
		platform *hivev1nutanix.MachinePool
		wantErrs []string
	}{
		{
			name: "valid values",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           4,
				NumCoresPerSocket: 2,
				MemoryMiB:         8192,
				DataDisks: []machinev1.NutanixVMDisk{
					{DiskSize: resource.MustParse("100Gi")},
				},
			},
			wantErrs: nil,
		},
		{
			name: "zero vCPUs",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           0,
				NumCoresPerSocket: 2,
				MemoryMiB:         8192,
			},
			wantErrs: []string{"numCPUs"},
		},
		{
			name: "zero cores per socket",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           4,
				NumCoresPerSocket: 0,
				MemoryMiB:         8192,
			},
			wantErrs: []string{"numCoresPerSocket"},
		},
		{
			name: "zero memory",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           4,
				NumCoresPerSocket: 2,
				MemoryMiB:         0,
			},
			wantErrs: []string{"memoryMiB"},
		},
		{
			name: "zero disk size",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           4,
				NumCoresPerSocket: 2,
				MemoryMiB:         8192,
				DataDisks: []machinev1.NutanixVMDisk{
					{DiskSize: resource.MustParse("0Gi")},
				},
			},
			wantErrs: []string{"diskSizeBytes"},
		},
		{
			name: "multiple invalid fields",
			platform: &hivev1nutanix.MachinePool{
				NumCPUs:           0,
				NumCoresPerSocket: 0,
				MemoryMiB:         0,
				DataDisks: []machinev1.NutanixVMDisk{
					{DiskSize: resource.MustParse("0Gi")},
				},
			},
			wantErrs: []string{"numCPUs", "numCoresPerSocket", "memoryMiB", "diskSizeBytes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateNutanixMachinePoolPlatformInvariants(tt.platform, field.NewPath("platform"))
			if len(tt.wantErrs) != len(errs) {
				t.Errorf("expected %d errors, got %d: %+v", len(tt.wantErrs), len(errs), errs)
			}
			for _, want := range tt.wantErrs {
				found := false
				for _, got := range errs {
					if strings.Contains(got.Field, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected error on field %q not found in errs: %+v", want, errs)
				}
			}
		})
	}
}

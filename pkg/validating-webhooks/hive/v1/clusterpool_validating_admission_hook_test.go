package v1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
)

func clusterPoolTemplate() *hivev1.ClusterPool {
	return &hivev1.ClusterPool{
		Spec: hivev1.ClusterPoolSpec{
			BaseDomain: "example.com",
		},
	}
}
func validGCPClusterPool() *hivev1.ClusterPool {
	cp := clusterPoolTemplate()
	cp.Spec.Platform.GCP = &hivev1gcp.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "us-central1",
	}
	return cp
}

func validAWSClusterPool() *hivev1.ClusterPool {
	cp := clusterPoolTemplate()
	cp.Spec.Platform.AWS = &hivev1aws.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "test-region",
	}
	return cp
}

func validAzureClusterPool() *hivev1.ClusterPool {
	cp := clusterPoolTemplate()
	cp.Spec.Platform.Azure = &hivev1azure.Platform{
		CredentialsSecretRef:        corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:                      "test-region",
		BaseDomainResourceGroupName: "os4-common",
	}
	return cp
}

func invalidOpenStackClusterPool() *hivev1.ClusterPool {
	cp := clusterPoolTemplate()
	cp.Spec.Platform.OpenStack = &hivev1openstack.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
	}
	return cp
}

func TestClusterPoolInitialize(t *testing.T) {
	data := NewClusterPoolValidatingAdmissionHook(*createDecoder(t))
	err := data.Initialize(nil, nil)
	assert.Nil(t, err)
}

func TestClusterPoolValidate(t *testing.T) {
	cases := []struct {
		name            string
		newObject       *hivev1.ClusterPool
		newObjectRaw    []byte
		oldObject       *hivev1.ClusterPool
		oldObjectRaw    []byte
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		gvr             *metav1.GroupVersionResource
	}{
		{
			name:            "Test valid create",
			newObject:       validAWSClusterPool(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is allowed with same data",
			oldObject:       validAWSClusterPool(),
			newObject:       validAWSClusterPool(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:      "Test Update Operation is allowed",
			oldObject: validAWSClusterPool(),
			newObject: func() *hivev1.ClusterPool {
				pool := validAWSClusterPool()
				pool.Spec.BaseDomain = "anotherbasedomain.example.com"
				return pool
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
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
			name: "Test doesn't validate with right version and resource, but wrong group",
			gvr: &metav1.GroupVersionResource{
				Group:    "not the right group",
				Version:  "v1",
				Resource: "clusterpools",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and resource, wrong version",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "not the right version",
				Resource: "clusterpools",
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
		{
			name:            "Azure create valid",
			newObject:       validAzureClusterPool(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "OpenStack unsupported platform",
			newObject:       invalidOpenStackClusterPool(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Azure create missing region",
			newObject: func() *hivev1.ClusterPool {
				cd := validAzureClusterPool()
				cd.Spec.Platform.Azure.Region = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Azure create missing credentials",
			newObject: func() *hivev1.ClusterPool {
				cd := validAzureClusterPool()
				cd.Spec.Platform.Azure.CredentialsSecretRef.Name = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Azure create missing baseDomainResourceGroupName",
			newObject: func() *hivev1.ClusterPool {
				cd := validAzureClusterPool()
				cd.Spec.Platform.Azure.BaseDomainResourceGroupName = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "create with two cloud platforms",
			newObject: func() *hivev1.ClusterPool {
				cd := validAzureClusterPool()
				awsCD := validAWSClusterPool()
				cd.Spec.Platform.AWS = awsCD.Spec.Platform.AWS
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "create with no cloud platforms",
			newObject: func() *hivev1.ClusterPool {
				cd := validAzureClusterPool()
				cd.Spec.Platform.Azure = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "valid GCP clusterdeployment",
			newObject:       validGCPClusterPool(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test valid delete",
			oldObject:       validAWSClusterPool(),
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name: "valid create with manifests secret",
			newObject: func() *hivev1.ClusterPool {
				cp := validAWSClusterPool()
				cp.Spec.ManifestsSecretRef = &corev1.LocalObjectReference{Name: "test-manifests-secret"}
				return cp
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:      "valid update with manifests secret",
			oldObject: validAWSClusterPool(),
			newObject: func() *hivev1.ClusterPool {
				cp := validAWSClusterPool()
				cp.Spec.ManifestsSecretRef = &corev1.LocalObjectReference{Name: "test-manifests-secret"}
				return cp
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := ClusterPoolValidatingAdmissionHook{
				decoder: *createDecoder(t),
			}

			if tc.gvr == nil {
				tc.gvr = &metav1.GroupVersionResource{
					Group:    "hive.openshift.io",
					Version:  "v1",
					Resource: "clusterpools",
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
			if !assert.Equal(t, tc.expectedAllowed, response.Allowed) {
				t.Logf("Response result = %#v", response.Result)
			}
		})
	}
}

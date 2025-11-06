package v1

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

func Test_ClusterProvisionAdmission_Validate_Kind(t *testing.T) {
	cases := []struct {
		name         string
		group        string
		version      string
		resource     string
		expectToSkip bool
	}{
		{
			name:     "clusterprovision",
			group:    clusterProvisionGroup,
			version:  clusterProvisionVersion,
			resource: clusterProvisionResource,
		},
		{
			name:         "different group",
			group:        "other group",
			version:      clusterProvisionVersion,
			resource:     clusterProvisionResource,
			expectToSkip: true,
		},
		{
			name:         "different version",
			group:        clusterProvisionGroup,
			version:      "other version",
			resource:     clusterProvisionResource,
			expectToSkip: true,
		},
		{
			name:         "different resource",
			group:        clusterProvisionGroup,
			version:      clusterProvisionVersion,
			resource:     "other resource",
			expectToSkip: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewClusterProvisionValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    tc.group,
					Version:  tc.version,
					Resource: tc.resource,
				},
				Operation: admissionv1beta1.Create,
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectToSkip, response.Allowed)
		})
	}
}

func Test_ClusterProvisionAdmission_Validate_Operation(t *testing.T) {
	cases := []struct {
		name         string
		operation    admissionv1beta1.Operation
		expectToSkip bool
	}{
		{
			name:      "create",
			operation: admissionv1beta1.Create,
		},
		{
			name:      "update",
			operation: admissionv1beta1.Update,
		},
		{
			name:         "other",
			operation:    admissionv1beta1.Operation("other"),
			expectToSkip: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewClusterProvisionValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    clusterProvisionGroup,
					Version:  clusterProvisionVersion,
					Resource: clusterProvisionResource,
				},
				Operation: tc.operation,
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectToSkip, response.Allowed)
		})
	}
}

func Test_ClusterProvisionAdmission_Validate_Create(t *testing.T) {
	cases := []struct {
		name          string
		provision     *hivev1.ClusterProvision
		expectAllowed bool
	}{
		{
			name:          "good",
			provision:     testClusterProvision(),
			expectAllowed: true,
		},
		{
			name: "missing clusterdeployment name",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.ClusterDeploymentRef.Name = ""
				return p
			}(),
		},
		{
			name: "negative attempt count",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Attempt = -1
				return p
			}(),
		},
		{
			name: "positive attempt count",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Attempt = 1
				return p
			}(),
			expectAllowed: true,
		},
		{
			name:          "complete stage",
			provision:     testCompletedClusterProvision(),
			expectAllowed: true,
		},
		{
			name: "failed stage",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Stage = hivev1.ClusterProvisionStageFailed
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "bad stage",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Stage = hivev1.ClusterProvisionStage("bad stage")
				return p
			}(),
		},
		{
			name: "absent previous cluster ID",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.PrevClusterID = nil
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "absent previous infra ID",
			provision: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.PrevInfraID = nil
				return p
			}(),
			expectAllowed: true,
		},
		{
			name:          "valid pre-installed",
			provision:     testPreInstalledClusterProvision(),
			expectAllowed: true,
		},
		{
			name: "attempt set for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.Attempt = 1
				return p
			}(),
		},
		{
			name: "invalid stage for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.Stage = hivev1.ClusterProvisionStageInitializing
				return p
			}(),
		},
		{
			name: "missing infra ID for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.InfraID = nil
				return p
			}(),
		},
		{
			name: "missing admin kubeconfig for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.AdminKubeconfigSecretRef = nil
				return p
			}(),
		},
		{
			name: "missing admin password for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.AdminPasswordSecretRef = nil
				return p
			}(),
		},
		{
			name: "prev cluster ID set for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.PrevClusterID = ptr.To("test-cluster-id")
				return p
			}(),
		},
		{
			name: "prev infra ID set for pre-installed",
			provision: func() *hivev1.ClusterProvision {
				p := testPreInstalledClusterProvision()
				p.Spec.PrevInfraID = ptr.To("test-infra-id")
				return p
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewClusterProvisionValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			rawProvision, err := json.Marshal(tc.provision)
			if !assert.NoError(t, err, "unexpected error marshalling provision") {
				return
			}
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    clusterProvisionGroup,
					Version:  clusterProvisionVersion,
					Resource: clusterProvisionResource,
				},
				Operation: admissionv1beta1.Create,
				Object:    runtime.RawExtension{Raw: rawProvision},
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectAllowed, response.Allowed, "unexpected response: %#v", response)
		})
	}
}

func Test_ClusterProvisionAdmission_Validate_Update(t *testing.T) {
	cases := []struct {
		name          string
		old           *hivev1.ClusterProvision
		new           *hivev1.ClusterProvision
		expectAllowed bool
	}{
		{
			name:          "no changes for provisioning",
			old:           testClusterProvision(),
			new:           testClusterProvision(),
			expectAllowed: true,
		},
		{
			name:          "no changes for completed",
			old:           testCompletedClusterProvision(),
			new:           testCompletedClusterProvision(),
			expectAllowed: true,
		},
		{
			name: "change clusterdeployment name",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.ClusterDeploymentRef.Name = "new-deployment"
				return p
			}(),
		},
		{
			name: "change pod spec",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.PodSpec.Containers[0].Name = "new-container"
				return p
			}(),
		},
		{
			name: "change attempt count",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Attempt = 1
				return p
			}(),
		},
		{
			name: "bad stage",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.Stage = hivev1.ClusterProvisionStage("bad stage")
				return p
			}(),
		},
		{
			name: "set cluster ID",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.ClusterID = ptr.To("new-cluster-id")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change cluster ID",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.ClusterID = ptr.To("new-cluster-id")
				return p
			}(),
		},
		{
			name: "set infra ID",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.InfraID = ptr.To("new-infra-id")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change infra ID",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.InfraID = ptr.To("new-infra-id")
				return p
			}(),
		},
		{
			name: "set install log",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.InstallLog = ptr.To("new-install-log")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change install log",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.InstallLog = ptr.To("new-install-log")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "set metadata",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.MetadataJSON = []byte("\"new-metadata\"")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change metadata",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.MetadataJSON = []byte("\"new-metadata\"")
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "set admin kubeconfig",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: "new-kubeconfig"}
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change admin kubeconfig",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: "new-kubeconfig"}
				return p
			}(),
		},
		{
			name: "set admin password",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: "new-password"}
				return p
			}(),
			expectAllowed: true,
		},
		{
			name: "change admin password",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: "new-password"}
				return p
			}(),
		},
		{
			name: "set previous cluster ID",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.PrevClusterID = ptr.To("new-prev-cluster-id")
				return p
			}(),
		},
		{
			name: "change previous cluster ID",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.PrevClusterID = ptr.To("new-prev-cluster-id")
				return p
			}(),
		},
		{
			name: "set previous infra ID",
			old:  testClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testClusterProvision()
				p.Spec.PrevInfraID = ptr.To("new-prev-infra-id")
				return p
			}(),
		},
		{
			name: "change previous infra ID",
			old:  testCompletedClusterProvision(),
			new: func() *hivev1.ClusterProvision {
				p := testCompletedClusterProvision()
				p.Spec.PrevInfraID = ptr.To("new-prev-infra-id")
				return p
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := NewClusterProvisionValidatingAdmissionHook(*createDecoder())
			cut.Initialize(nil, nil)
			oldAsJSON, err := json.Marshal(tc.old)
			if !assert.NoError(t, err, "unexpected error marshalling old provision") {
				return
			}
			newAsJSON, err := json.Marshal(tc.new)
			if !assert.NoError(t, err, "unexpected error marshalling new provision") {
				return
			}
			request := &admissionv1beta1.AdmissionRequest{
				Resource: metav1.GroupVersionResource{
					Group:    clusterProvisionGroup,
					Version:  clusterProvisionVersion,
					Resource: clusterProvisionResource,
				},
				Operation: admissionv1beta1.Update,
				Object:    runtime.RawExtension{Raw: newAsJSON},
				OldObject: runtime.RawExtension{Raw: oldAsJSON},
			}
			response := cut.Validate(request)
			assert.Equal(t, tc.expectAllowed, response.Allowed, "unexpected response: %#v", response.Result)
		})
	}
}

func Test_ClusterProvisionAdmission_Validate_Update_StageTransition(t *testing.T) {
	expectedAcceptedTransitions := []struct {
		from hivev1.ClusterProvisionStage
		to   hivev1.ClusterProvisionStage
	}{
		{
			from: hivev1.ClusterProvisionStageInitializing,
			to:   hivev1.ClusterProvisionStageProvisioning,
		},
		{
			from: hivev1.ClusterProvisionStageProvisioning,
			to:   hivev1.ClusterProvisionStageComplete,
		},
		{
			from: hivev1.ClusterProvisionStageInitializing,
			to:   hivev1.ClusterProvisionStageFailed,
		},
		{
			from: hivev1.ClusterProvisionStageProvisioning,
			to:   hivev1.ClusterProvisionStageFailed,
		},
	}
	for oldStage := range validProvisionStages {
		for newStage := range validProvisionStages {
			t.Run(
				fmt.Sprintf("%s to %s", oldStage, newStage),
				func(t *testing.T) {
					cut := NewClusterProvisionValidatingAdmissionHook(*createDecoder())
					cut.Initialize(nil, nil)
					oldProvision := testCompletedClusterProvision()
					oldProvision.Spec.Stage = oldStage
					oldAsJSON, err := json.Marshal(oldProvision)
					if !assert.NoError(t, err, "unexpected error marshalling old provision") {
						return
					}
					newProvision := testCompletedClusterProvision()
					newProvision.Spec.Stage = newStage
					newAsJSON, err := json.Marshal(newProvision)
					if !assert.NoError(t, err, "unexpected error marshalling new provision") {
						return
					}
					request := &admissionv1beta1.AdmissionRequest{
						Resource: metav1.GroupVersionResource{
							Group:    clusterProvisionGroup,
							Version:  clusterProvisionVersion,
							Resource: clusterProvisionResource,
						},
						Operation: admissionv1beta1.Update,
						Object:    runtime.RawExtension{Raw: newAsJSON},
						OldObject: runtime.RawExtension{Raw: oldAsJSON},
					}
					response := cut.Validate(request)
					expectedAllowed := oldStage == newStage
					for _, t := range expectedAcceptedTransitions {
						if oldStage == t.from && newStage == t.to {
							expectedAllowed = true
						}
					}
					assert.Equal(t, expectedAllowed, response.Allowed, "unexpected response")
				},
			)
		}
	}
}

func testClusterProvision() *hivev1.ClusterProvision {
	return &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-provision",
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: "test-deployment",
			},
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "test-image",
					},
				},
			},
			Attempt:       0,
			Stage:         hivev1.ClusterProvisionStageInitializing,
			PrevClusterID: ptr.To("test-prev-cluster-id"),
			PrevInfraID:   ptr.To("test-prev-infra-id"),
		},
	}
}

func testCompletedClusterProvision() *hivev1.ClusterProvision {
	provision := testClusterProvision()
	provision.Spec.Stage = hivev1.ClusterProvisionStageComplete
	provision.Spec.ClusterID = ptr.To("test-cluster-id")
	provision.Spec.InfraID = ptr.To("test-infra-id")
	provision.Spec.InstallLog = ptr.To("test-install-log")
	provision.Spec.MetadataJSON = []byte("\"test-metadata\"")
	provision.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: "test-admin-kubeconfig"}
	provision.Spec.AdminPasswordSecretRef = &corev1.LocalObjectReference{Name: "test-admin-password"}
	provision.Spec.PrevClusterID = ptr.To("test-prev-cluster-id")
	provision.Spec.PrevInfraID = ptr.To("test-prev-infra-id")
	return provision
}

func testPreInstalledClusterProvision() *hivev1.ClusterProvision {
	return &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-provision",
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: "test-deployment",
			},
			Stage:                    hivev1.ClusterProvisionStageComplete,
			ClusterID:                ptr.To("test-prev-cluster-id"),
			InfraID:                  ptr.To("test-prev-infra-id"),
			AdminKubeconfigSecretRef: &corev1.LocalObjectReference{Name: "test-admin-kubeconfig"},
			AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: "test-admin-password"},
		},
	}
}

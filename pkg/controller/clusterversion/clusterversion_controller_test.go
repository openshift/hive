package clusterversion

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testName                        = "foo-lqmsh"
	testClusterName                 = "bar"
	testClusterID                   = "testFooClusterUUID"
	testNamespace                   = "default"
	pullSecretSecret                = "pull-secret"
	testRemoteClusterCurrentVersion = "4.0.0"
	remoteClusterVersionObjectName  = "version"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterVersionReconcile(t *testing.T) {

	tests := []struct {
		name                 string
		existing             []runtime.Object
		clusterVersionStatus *configv1.ClusterVersionStatus
		noRemoteCall         bool
		expectError          bool
		validate             func(*testing.T, *hivev1.ClusterDeployment)
	}{
		{
			// no cluster deployment, no error expected
			name:         "clusterdeployment doesn't exist",
			noRemoteCall: true,
		},
		{
			name: "deleted clusterdeployment",
			existing: []runtime.Object{
				testClusterDeployment(deleted()),
			},
			noRemoteCall: true,
		},
		{
			name: "version in labels",
			existing: []runtime.Object{
				testClusterDeployment(),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(withVersion("2.3.4+somebuild")),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Equal(t, "2.3.4+somebuild", cd.Labels[constants.VersionLabel], "unexpected version label")
				assert.Equal(t, "2", cd.Labels[constants.VersionMajorLabel], "unexpected version major label")
				assert.Equal(t, "2.3", cd.Labels[constants.VersionMajorMinorLabel], "unexpected version major-minor label")
				assert.Equal(t, "2.3.4", cd.Labels[constants.VersionMajorMinorPatchLabel], "unexpected version major-minor-patch label")
			},
		},
		{
			name: "upgradeable condition true",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(map[string]string{
						constants.MinorVersionUpgradeUnavailable: "Can't upgrade",
					}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorUpgradeable,
						Status: configv1.ConditionTrue,
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition false",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(map[string]string{
						constants.MinorVersionUpgradeUnavailable: "Can't upgrade",
					}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorUpgradeable,
						Status: configv1.ConditionFalse,
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "Upgradeable: False", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition true with annotation not present",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorUpgradeable,
						Status: configv1.ConditionTrue,
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition false with annotation not present",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Message: "Can't do the upgrade",
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "Can't do the upgrade", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition false with message",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(
						map[string]string{constants.MinorVersionUpgradeUnavailable: "Can't upgrade"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Message: "Can't do the upgrade",
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "Can't do the upgrade", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition unknown",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(
						map[string]string{constants.MinorVersionUpgradeUnavailable: "Can't upgrade"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:   configv1.OperatorUpgradeable,
						Status: configv1.ConditionUnknown,
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "Upgradeable: Unknown", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition unknown with message",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(
						map[string]string{constants.MinorVersionUpgradeUnavailable: "Can't upgrade"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionUnknown,
						Message: "Can't read status",
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "Can't read status", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "upgradeable condition when multiple conditions",
			existing: []runtime.Object{
				testClusterDeployment(
					withLabels(map[string]string{
						constants.VersionLabel:                "1.2.3",
						constants.VersionMajorLabel:           "1",
						constants.VersionMajorMinorLabel:      "1.2",
						constants.VersionMajorMinorPatchLabel: "1.2.3",
					}),
					withAnnotations(
						map[string]string{constants.MinorVersionUpgradeUnavailable: "Can't upgrade"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(
				withVersion("1.2.3"),
				withConditions(
					configv1.ClusterOperatorStatusCondition{
						Type: configv1.OperatorProgressing,
					},
					configv1.ClusterOperatorStatusCondition{
						Type:    configv1.OperatorUpgradeable,
						Status:  configv1.ConditionFalse,
						Message: "It can't upgrade",
					}),
			),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				value, ok := cd.Annotations[constants.MinorVersionUpgradeUnavailable]
				assert.True(t, ok, "value for annotation hive.openshift.io/minor-version-upgrade-unavailable is missing")
				assert.Equal(t, "It can't upgrade", value, "unexpected value for annotation hive.openshift.io/minor-version-upgrade-unavailable")
			},
		},
		{
			name: "set ClusterVersionStatus",
			existing: []runtime.Object{
				testClusterDeployment(withAnnotations(map[string]string{constants.SyncClusterVersionStatusAnnotation: "true"})),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				if assert.NotNil(t, cd.Status.ClusterVersionStatus) {
					assert.Equal(t, testRemoteClusterVersionStatus(), cd.Status.ClusterVersionStatus, "unexpected cd.status.clusterVersionStatus")
				}
			},
		},
		{
			name: "clear ClusterVersionStatus",
			existing: []runtime.Object{
				testClusterDeployment(
					withClusterVersionStatus(testRemoteClusterVersionStatus()),
					withAnnotations(map[string]string{constants.SyncClusterVersionStatusAnnotation: "false"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Nil(t, cd.Status.ClusterVersionStatus, "expected cd.status.clusterVersionStatus to be nil")
			},
		},
		{
			name: "changed ClusterVersionStatus",
			existing: []runtime.Object{
				testClusterDeployment(
					withClusterVersionStatus(testRemoteClusterVersionStatus()),
					withAnnotations(map[string]string{constants.SyncClusterVersionStatusAnnotation: "true"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(withConditions(
				configv1.ClusterOperatorStatusCondition{
					Type:   configv1.OperatorDegraded,
					Status: configv1.ConditionTrue,
				},
			)),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				if assert.NotNil(t, cd.Status.ClusterVersionStatus) {
					assert.Equal(t, testRemoteClusterVersionStatus(withConditions(
						configv1.ClusterOperatorStatusCondition{
							Type:   configv1.OperatorDegraded,
							Status: configv1.ConditionTrue,
						},
					)), cd.Status.ClusterVersionStatus, "unexpected cd.status.clusterVersionStatus")
				}
			},
		},
		{
			name: "unchanged populated ClusterVersionStatus",
			existing: []runtime.Object{
				testClusterDeployment(
					withClusterVersionStatus(testRemoteClusterVersionStatus()),
					withAnnotations(map[string]string{constants.SyncClusterVersionStatusAnnotation: "true"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				if assert.NotNil(t, cd.Status.ClusterVersionStatus) {
					assert.Equal(t, testRemoteClusterVersionStatus(), cd.Status.ClusterVersionStatus, "unexpected cd.status.clusterVersionStatus")
				}

			},
		},
		{
			name: "unchanged unset ClusterVersionStatus",
			existing: []runtime.Object{
				testClusterDeployment(
					withClusterVersionStatus(testRemoteClusterVersionStatus()),
					// Also test unparseable annotation here
					withAnnotations(map[string]string{constants.SyncClusterVersionStatusAnnotation: "bogus"}),
				),
				testKubeconfigSecret(),
			},
			clusterVersionStatus: testRemoteClusterVersionStatus(),
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				assert.Nil(t, cd.Status.ClusterVersionStatus)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			mockCtrl := gomock.NewController(t)
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			if !test.noRemoteCall {
				mockRemoteClientBuilder.EXPECT().Build().Return(testRemoteClusterAPIClient(test.clusterVersionStatus), nil)
			}
			rcd := &ReconcileClusterVersion{
				Client:                        fakeClient,
				scheme:                        scheme,
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			_, err := rcd.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespacedName})

			if test.validate != nil {
				cd := &hivev1.ClusterDeployment{}
				err := fakeClient.Get(context.TODO(), namespacedName, cd)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				test.validate(t, cd)
			}

			if err != nil && !test.expectError {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && test.expectError {
				t.Errorf("Expected error but got none")
			}
		})
	}
}

type cdOpt func(*hivev1.ClusterDeployment)

func withLabels(labels map[string]string) cdOpt {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Labels = labels
	}
}

func withAnnotations(annotations map[string]string) cdOpt {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Annotations = annotations
	}
}

func deleted() cdOpt {
	return func(cd *hivev1.ClusterDeployment) {
		now := metav1.Now()
		cd.DeletionTimestamp = &now
	}
}

func withClusterVersionStatus(cvs *configv1.ClusterVersionStatus) cdOpt {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Status.ClusterVersionStatus = cvs
	}
}

func testClusterDeployment(opts ...cdOpt) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
			PullSecretRef: &corev1.LocalObjectReference{
				Name: pullSecretSecret,
			},
			Platform: hivev1.Platform{
				AWS: &hivev1aws.Platform{
					CredentialsSecretRef: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
					Region: "us-east-1",
				},
			},
			ClusterMetadata: &hivev1.ClusterMetadata{
				ClusterID: testClusterID,
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{
					Name: "kubeconfig-secret",
				},
			},
			Installed: true,
		},
		Status: hivev1.ClusterDeploymentStatus{
			Conditions: []hivev1.ClusterDeploymentCondition{{
				Type:   hivev1.UnreachableCondition,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	for _, opt := range opts {
		opt(cd)
	}
	return cd
}

func testKubeconfigSecret() *corev1.Secret {
	return testSecret("kubeconfig-secret", "kubeconfig", "KUBECONFIG-DATA")
}

func testSecret(name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return s
}

func testRemoteClusterAPIClient(status *configv1.ClusterVersionStatus) client.Client {
	remoteClusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: remoteClusterVersionObjectName,
		},
	}
	remoteClusterVersion.Status = *status
	return testfake.NewFakeClientBuilder().WithRuntimeObjects(remoteClusterVersion).Build()
}

type cvsOpt func(*configv1.ClusterVersionStatus)

func withConditions(conditions ...configv1.ClusterOperatorStatusCondition) cvsOpt {
	return func(cvs *configv1.ClusterVersionStatus) {
		cvs.Conditions = conditions
	}
}

func withVersion(version string) cvsOpt {
	return func(cvs *configv1.ClusterVersionStatus) {
		cvs.Desired.Version = version
	}
}

func testRemoteClusterVersionStatus(opts ...cvsOpt) *configv1.ClusterVersionStatus {
	zeroTime := metav1.NewTime(time.Unix(0, 0))
	cvs := &configv1.ClusterVersionStatus{
		History: []configv1.UpdateHistory{
			{
				State:          configv1.CompletedUpdate,
				Version:        testRemoteClusterCurrentVersion,
				Image:          "TESTIMAGE",
				CompletionTime: &zeroTime,
			},
		},
		ObservedGeneration: 123456789,
		VersionHash:        "TESTVERSIONHASH",
	}
	for _, opt := range opts {
		opt(cvs)
	}
	return cvs
}

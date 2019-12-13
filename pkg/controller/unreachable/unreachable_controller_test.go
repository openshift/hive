/*
Copyright (C) 2019 Red Hat, Inc.

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
package unreachable

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	remoteclientmock "github.com/openshift/hive/pkg/remoteclient/mock"
)

const (
	testName         = "foo-unreachable"
	testClusterName  = "bar"
	testClusterID    = "testFooClusterUUID"
	testNamespace    = "default"
	pullSecretSecret = "pull-secret"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	configv1.Install(scheme.Scheme)

	tests := []struct {
		name                          string
		existing                      []runtime.Object
		errorConnecting               bool
		errorConnectingSecondary      *bool
		expectError                   bool
		expectNoCondition             bool
		expectedStatus                corev1.ConditionStatus
		expectActiveOverrideCondition bool
		expectedActiveOverrideStatus  corev1.ConditionStatus
	}{
		{
			name: "unreachable condition should be added",
			existing: []runtime.Object{
				getClusterDeployment(),
			},
			errorConnecting: true,
			expectedStatus:  corev1.ConditionTrue,
		},
		{
			name: "unreachable condition should change from false to true",
			existing: []runtime.Object{
				getCDWithUnreachableConditionFalse(),
			},
			errorConnecting: true,
			expectedStatus:  corev1.ConditionTrue,
		},
		{
			name: "lastProbeTime>2hr lastTransitionTime-12hour condition false to true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Minute * 121)))
					lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Hour * 12)))
					return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
				}(),
			},
			errorConnecting: true,
			expectedStatus:  corev1.ConditionTrue,
		},
		{
			name: "unreachable condition should not be added",
			existing: []runtime.Object{
				getClusterDeployment(),
			},
			expectNoCondition: true,
		},
		{
			name: "unreachable condition should change from true to false",
			existing: []runtime.Object{
				getCDWithUnreachableConditionTrue(),
			},
			expectedStatus: corev1.ConditionFalse,
		},
		{
			name: "lastProbeTime lastTransitionTime 1hour ago unreachable condition true to false",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Hour * 1)))
					lastTransitionTime := lastProbeTime
					return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionTrue)
				}(),
			},
			expectedStatus: corev1.ConditionFalse,
		},
		{
			name: "primary reachable",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := getCDWithUnreachableConditionTrue()
					cd.Spec.ControlPlaneConfig.APIURLOverride = "test override URL"
					return cd
				}(),
			},
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionTrue,
		},
		{
			name: "secondary reachable",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := getCDWithUnreachableConditionTrue()
					cd.Spec.ControlPlaneConfig.APIURLOverride = "test override URL"
					return cd
				}(),
			},
			errorConnecting:               true,
			errorConnectingSecondary:      pointer.BoolPtr(false),
			expectedStatus:                corev1.ConditionFalse,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
		},
		{
			name: "primary and secondary unreachable",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := getClusterDeployment()
					cd.Spec.ControlPlaneConfig.APIURLOverride = "test override URL"
					return cd
				}(),
			},
			errorConnecting:               true,
			errorConnectingSecondary:      pointer.BoolPtr(true),
			expectedStatus:                corev1.ConditionTrue,
			expectActiveOverrideCondition: true,
			expectedActiveOverrideStatus:  corev1.ConditionFalse,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockRemoteClientBuilder := remoteclientmock.NewMockBuilder(mockCtrl)
			mockRemoteClientBuilder.EXPECT().UsePrimaryAPIURL().Return(mockRemoteClientBuilder)
			var buildError error
			if test.errorConnecting {
				buildError = errors.New("cluster not reachable")
			}
			mockRemoteClientBuilder.EXPECT().Build().Return(nil, buildError)
			if test.errorConnectingSecondary != nil {
				mockRemoteClientBuilder.EXPECT().UseSecondaryAPIURL().Return(mockRemoteClientBuilder)
				var buildError error
				if *test.errorConnectingSecondary {
					buildError = errors.New("cluster not reachable")
				}
				mockRemoteClientBuilder.EXPECT().Build().Return(nil, buildError)
			}
			rcd := &ReconcileRemoteMachineSet{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "unreachable"),
				remoteClusterAPIClientBuilder: func(*hivev1.ClusterDeployment) remoteclient.Builder { return mockRemoteClientBuilder },
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			_, err := rcd.Reconcile(reconcile.Request{NamespacedName: namespacedName})

			if test.expectError {
				assert.Error(t, err, "expected error during reconcile")
				return
			}

			assert.NoError(t, err, "unexpected error during reconcile")
			cd := &hivev1.ClusterDeployment{}
			if err := fakeClient.Get(context.TODO(), namespacedName, cd); assert.NoError(t, err, "missing clusterdeployment") {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if test.expectNoCondition {
					assert.Nil(t, cond, "expected no unreachable condition")
				} else {
					if assert.NotNil(t, cond, "missing unreachable condition") {
						assert.Equal(t, string(test.expectedStatus), string(cond.Status), "unexpected status on unreachable condition")
					}
				}
				cond = controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ActiveAPIURLOverrideCondition)
				if !test.expectActiveOverrideCondition {
					assert.Nil(t, cond, "expected no active override condition")
				} else {
					if assert.NotNil(t, cond, "missing active override condition") {
						assert.Equal(t, string(test.expectedActiveOverrideStatus), string(cond.Status), "unexpected status on active override condition")
					}
				}
			}
		})
	}
}

func TestIsTimeForConnectivityCheck(t *testing.T) {
	tests := []struct {
		name     string
		cd       *hivev1.ClusterDeployment
		expected bool
	}{
		{
			name: "lastProbeTime, lastTransitionTime is same",
			cd: func() *hivev1.ClusterDeployment {
				lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Hour * 1)))
				lastTransitionTime := lastProbeTime
				return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
			}(),
			expected: true,
		},
		{
			name: "lastProbeTime 20Mins ago and lastTransitionTime 100Mins ago",
			cd: func() *hivev1.ClusterDeployment {
				lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Minute * 20)))
				lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Minute * 100)))
				return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
			}(),
			expected: false,
		},
		{
			name: "lastProbeTime 1hour ago lastTransitionTime 5hour ago",
			cd: func() *hivev1.ClusterDeployment {
				lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Hour * 1)))
				lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Hour * 5)))
				return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
			}(),
			expected: false,
		},
		{
			name: "lastProbeTime more than 2hour ago lastTransitionTime 12hour ago",
			cd: func() *hivev1.ClusterDeployment {
				lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Minute * 121)))
				lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Hour * 12)))
				return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
			}(),
			expected: true,
		},
		{
			name: "lastProbeTime 60min ago lastTransitionTime 160min ago",
			cd: func() *hivev1.ClusterDeployment {
				lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Minute * 60)))
				lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Minute * 160)))
				return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
			}(),
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cond := controllerutils.FindClusterDeploymentCondition(test.cd.Status.Conditions, hivev1.UnreachableCondition)
			actual := isTimeForConnectivityCheck(cond)
			if test.expected != actual {
				t.Errorf("expected: %t actual: %t", test.expected, actual)
			}
		})
	}
}

func getClusterDeployment() *hivev1.ClusterDeployment {
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
	}
	return cd
}

func getCDWithUnreachableConditionFalse() *hivev1.ClusterDeployment {
	cd := getClusterDeployment()
	probeTime := time.Now().Add(-(time.Hour * 1))
	lastProbeTime := metav1.NewTime(probeTime)

	// Explicitly set the condition hivev1.UnreachableCondition as false
	var conditions []hivev1.ClusterDeploymentCondition
	cd.Status.Conditions = append(
		conditions,
		hivev1.ClusterDeploymentCondition{
			Type:               hivev1.UnreachableCondition,
			Status:             corev1.ConditionFalse,
			Reason:             "ClusterReachable",
			Message:            "Setting the condition when cluster is reachable",
			LastTransitionTime: lastProbeTime,
			LastProbeTime:      lastProbeTime,
		},
	)
	return cd
}

func getCDWithUnreachableConditionTrue() *hivev1.ClusterDeployment {
	cd := getClusterDeployment()
	probeTime := time.Now().Add(-(time.Hour * 1))
	lastProbeTime := metav1.NewTime(probeTime)

	// Explicitly set the condition hivev1.UnreachableCondition as false
	var conditions []hivev1.ClusterDeploymentCondition
	cd.Status.Conditions = append(
		conditions,
		hivev1.ClusterDeploymentCondition{
			Type:               hivev1.UnreachableCondition,
			Status:             corev1.ConditionTrue,
			Reason:             "ClusterReachable",
			Message:            "Setting the condition when cluster is reachable",
			LastTransitionTime: lastProbeTime,
			LastProbeTime:      lastProbeTime,
		},
	)
	return cd
}

func getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime metav1.Time, status corev1.ConditionStatus) *hivev1.ClusterDeployment {
	cd := getClusterDeployment()
	var conditions []hivev1.ClusterDeploymentCondition

	cd.Status.Conditions = append(
		conditions,
		hivev1.ClusterDeploymentCondition{
			Type:               hivev1.UnreachableCondition,
			Status:             status,
			Reason:             "test reason",
			Message:            "test message",
			LastTransitionTime: lastTransitionTime,
			LastProbeTime:      lastProbeTime,
		},
	)
	return cd
}

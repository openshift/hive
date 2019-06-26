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

	log "github.com/sirupsen/logrus"

	"github.com/openshift/hive/pkg/apis"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testName              = "foo-unreachable"
	testClusterName       = "bar"
	testClusterID         = "testFooClusterUUID"
	testNamespace         = "default"
	sshKeySecret          = "ssh-key"
	pullSecretSecret      = "pull-secret"
	adminKubeconfigSecret = "foo-unreachable-admin-kubeconfig"
	adminKubeconfig       = `clusters:
- cluster:
    certificate-authority-data: JUNK
    server: https://bar-api.clusters.example.com:6443
  name: bar
`
	testRemoteClusterCurrentVersion = "4.0.0"
	remoteClusterVersionObjectName  = "version"

	remoteClusterRouteObjectName      = "console"
	remoteClusterRouteObjectNamespace = "openshift-console"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestUnreachablClusterStatusCondition(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	configv1.Install(scheme.Scheme) //remove later
	tests := []struct {
		name        string
		existing    []runtime.Object
		expectError bool
		validate    func(*testing.T, *hivev1.ClusterDeployment)
	}{
		{
			name: "unreachable condition should be added",
			existing: []runtime.Object{
				getClusterDeployment(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond.Status != corev1.ConditionTrue {
					t.Errorf("Did not get expected state for unreachable condition. Expected: \n%#v\nGot: \n%#v", corev1.ConditionTrue, cond.Status)
				}
			},
		},
		{
			name: "unreachable condition should change from false to true",
			existing: []runtime.Object{
				getCDWithUnreachableConditionFalse(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond.Status != corev1.ConditionTrue {
					t.Errorf("Did not get expected state for unreachable condition. Expected: \n%#v\nGot: \n%#v", corev1.ConditionTrue, cond.Status)
				}
			},
		},
		{
			name: "lastProbeTime>2hr lastTransitionTime-12hour condition false to true",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Minute * 121)))
					lastTransitionTime := metav1.NewTime(time.Now().Add(-(time.Hour * 12)))
					return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionFalse)
				}(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond.Status != corev1.ConditionTrue {
					t.Errorf("Did not get expected state for unreachable condition. Expected: \n%#v\nGot: \n%#v", corev1.ConditionFalse, cond.Status)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			rcd := &ReconcileRemoteMachineSet{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "unreachable"),
				remoteClusterAPIClientBuilder: mockUnreachableClusterAPIClientBuilder,
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			_, err := rcd.Reconcile(reconcile.Request{NamespacedName: namespacedName})

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

func TestReachableClusterStatusCondition(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	configv1.Install(scheme.Scheme)

	tests := []struct {
		name        string
		existing    []runtime.Object
		expectError bool
		validate    func(*testing.T, *hivev1.ClusterDeployment)
	}{
		{
			name: "unreachable condition should not be added",
			existing: []runtime.Object{
				getClusterDeployment(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond != nil {
					t.Error("Did not expect to get the unreachable status condition.")
				}
			},
		},
		{
			name: "unreachable condition should change from true to false",
			existing: []runtime.Object{
				getCDWithUnreachableConditionTrue(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond.Status != corev1.ConditionFalse {
					t.Errorf("Did not get expected state for unreachable condition. Expected: \n%#v\nGot: \n%#v", corev1.ConditionFalse, cond.Status)
				}
			},
		},
		{
			name: "lastProbeTime lastTransitionTime 1hour ago unreachable condition true to false",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					lastProbeTime := metav1.NewTime(time.Now().Add(-(time.Hour * 1)))
					lastTransitionTime := lastProbeTime
					return getCDWithProbeAndTransitionTime(lastProbeTime, lastTransitionTime, corev1.ConditionTrue)
				}(),
				getKubeconfigSecret(),
			},
			validate: func(t *testing.T, cd *hivev1.ClusterDeployment) {
				cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.UnreachableCondition)
				if cond.Status != corev1.ConditionFalse {
					t.Errorf("Did not get expected state for unreachable condition. Expected: \n%#v\nGot: \n%#v", corev1.ConditionFalse, cond.Status)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			rcd := &ReconcileRemoteMachineSet{
				Client:                        fakeClient,
				scheme:                        scheme.Scheme,
				logger:                        log.WithField("controller", "unreachable"),
				remoteClusterAPIClientBuilder: mockReachableClusterAPIClientBuilder,
			}

			namespacedName := types.NamespacedName{
				Name:      testName,
				Namespace: testNamespace,
			}

			_, err := rcd.Reconcile(reconcile.Request{NamespacedName: namespacedName})

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
			name: "lastProbeTime 20Mins ago and lastTransitionTime10Mins ago",
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

func mockUnreachableClusterAPIClientBuilder(secretData, controllerName string) (client.Client, error) {
	err := errors.New("cluster not reachable")
	return nil, err
}

func mockReachableClusterAPIClientBuilder(secretData, controllerName string) (client.Client, error) {

	remoteClusterVersion := &configv1.ClusterVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: remoteClusterVersionObjectName,
		},
	}
	remoteClusterVersion.Status = testRemoteClusterVersionStatus()
	remoteClient := fake.NewFakeClient(remoteClusterVersion)
	return remoteClient, nil
}

func testRemoteClusterVersionStatus() configv1.ClusterVersionStatus {
	zeroTime := metav1.NewTime(time.Unix(0, 0))
	status := configv1.ClusterVersionStatus{
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
	return status
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
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: &corev1.LocalObjectReference{
				Name: pullSecretSecret,
			},
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: "us-east-1",
				},
			},
			Networking: hivev1.Networking{
				Type: hivev1.NetworkTypeOpenshiftSDN,
			},
			PlatformSecrets: hivev1.PlatformSecrets{
				AWS: &hivev1.AWSPlatformSecrets{
					Credentials: corev1.LocalObjectReference{
						Name: "aws-credentials",
					},
				},
			},
		},
		Status: hivev1.ClusterDeploymentStatus{
			ClusterID: testClusterID,
			AdminKubeconfigSecret: corev1.LocalObjectReference{
				Name: "kubeconfig-secret",
			},
		},
	}
	cd.Status.Installed = true
	return cd
}

func getCDWithUnreachableConditionFalse() *hivev1.ClusterDeployment {
	cd := getClusterDeployment()
	probeTime := time.Now().Add(-(time.Hour * 1))
	lastProbeTime := metav1.NewTime(probeTime)

	// Explictly set the condition hivev1.UnreachableCondition as false
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

	// Explictly set the condition hivev1.UnreachableCondition as false
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
			Reason:             "ClusterReachable",
			Message:            "Setting the condition when cluster is reachable",
			LastTransitionTime: lastTransitionTime,
			LastProbeTime:      lastProbeTime,
		},
	)
	return cd
}

func getKubeconfigSecret() *corev1.Secret {
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

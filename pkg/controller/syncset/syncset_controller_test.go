/*
Copyright 2018 The Kubernetes Authors.

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

package syncset

import (
	"context"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/resource"
)

const (
	testName                 = "foo"
	testNamespace            = "default"
	testClusterID            = "foo-12345-uuid"
	testInfraID              = "foo-12345"
	adminKubeconfigSecret    = "foo-admin-kubeconfig"
	adminKubeconfigSecretKey = "kubeconfig"
	sshKeySecret             = "foo-ssh-key"
	sshKeySecretKey          = "ssh-publickey"
	pullSecretSecret         = "foo-pull-secret"
	pullSecretSecretKey      = ".dockercfg"
	testAMI                  = "ami-totallyfake"
	kubeconfigTemplate       = `
apiVersion: v1
clusters:
- cluster:
    server: %s
  name: cluster1
contexts:
- context:
    cluster: cluster1
  name: context1
current-context: context1
kind: Config
preferences: {}
`
)

type FakeHelper struct {
}

func NewFakeHelper(kubeconfig []byte, logger log.FieldLogger) Applier {
	var helper Applier = &FakeHelper{}
	return helper
}

func (r *FakeHelper) Apply(obj []byte) error {
	return nil
}

func (r *FakeHelper) Info(obj []byte) (*resource.Info, error) {
	return &resource.Info{
		Name:       "foo",
		Namespace:  "foo",
		Kind:       "secret",
		APIVersion: "v1alpha1",
	}, nil
}

func (r *FakeHelper) Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType types.PatchType) error {
	return nil
}

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestSyncSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	// Utility function to get the test CD from the fake client
	getCD := func(c client.Client) *hivev1.ClusterDeployment {
		cd := &hivev1.ClusterDeployment{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: testName, Namespace: testNamespace}, cd)
		if err == nil {
			return cd
		}
		return nil
	}

	tests := []struct {
		name                  string
		existing              []runtime.Object
		expectErr             bool
		applySuccess          bool
		expectedSyncSetStatus []hivev1.SyncSetObjectStatus
	}{
		{
			name: "Kubeconfig doesn't exist yet",
			existing: []runtime.Object{
				testClusterDeployment(nil),
			},
			expectErr: true,
		},
		{
			name: "Create resource",
			existing: []runtime.Object{
				testClusterDeployment(nil),
				testKubeconfig(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSyncSet(),
			},
			applySuccess: true,
			expectedSyncSetStatus: []hivev1.SyncSetObjectStatus{
				{
					Name: testName,
					Resources: []hivev1.SyncStatus{
						{
							APIVersion: "v1alpha1",
							Kind:       "secret",
							Name:       testName,
							Namespace:  testName,
							Hash:       "cad915ea8edcb85f1ebb7a8f5e48baa7",
							Conditions: []hivev1.SyncCondition{
								{
									Type:   hivev1.ApplySuccessSyncCondition,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Update resource",
			existing: []runtime.Object{
				testClusterDeployment([]hivev1.SyncSetObjectStatus{
					{
						Name: testName,
						Resources: []hivev1.SyncStatus{
							{
								APIVersion: "v1alpha1",
								Kind:       "secret",
								Name:       testName,
								Namespace:  testName,
								Hash:       "79054025255fb1a26e4bc422aef54eb4",
								Conditions: []hivev1.SyncCondition{
									{
										Type:   hivev1.ApplySuccessSyncCondition,
										Status: corev1.ConditionTrue,
									},
								},
							},
						},
					},
				}),
				testKubeconfig(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSyncSet(),
			},
			applySuccess: true,
			expectedSyncSetStatus: []hivev1.SyncSetObjectStatus{
				{
					Name: testName,
					Resources: []hivev1.SyncStatus{
						{
							APIVersion: "v1alpha1",
							Kind:       "secret",
							Name:       testName,
							Namespace:  testName,
							Hash:       "cad915ea8edcb85f1ebb7a8f5e48baa7",
							Conditions: []hivev1.SyncCondition{
								{
									Type:   hivev1.ApplySuccessSyncCondition,
									Status: corev1.ConditionTrue,
								},
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			rcd := &ReconcileSyncSet{
				Client:         fakeClient,
				scheme:         scheme.Scheme,
				logger:         log.WithField("controller", "syncset"),
				applierBuilder: NewFakeHelper,
			}
			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})
			if err != nil && !test.expectErr {
				t.Errorf("unexpected error: %v", err)
			}
			if err == nil && test.expectErr {
				t.Errorf("expected error but got none")
			}

			if test.expectedSyncSetStatus != nil {
				cd := getCD(fakeClient)
				expectedSetSyncStatus := test.expectedSyncSetStatus[0]
				syncSetStatus := cd.Status.SyncSetStatus[0]
				for _, eRes := range expectedSetSyncStatus.Resources {
					found := false
					for _, res := range syncSetStatus.Resources {
						if eRes.Name == res.Name && eRes.Namespace == res.Namespace && eRes.Kind == res.Kind {
							found = true
							assert.Equal(t, eRes.Hash, res.Hash)
							for _, condition := range res.Conditions {
								if condition.Type == hivev1.ApplySuccessSyncCondition {
									if test.applySuccess {
										assert.Equal(t, condition.Status, corev1.ConditionTrue)
									}
								}
							}
						}
					}
					if !found {
						t.Errorf("did not find expeceted resource status: %v", eRes.Name)
					}
				}
			}
		})
	}
}

func testClusterDeployment(syncSetStatus []hivev1.SyncSetObjectStatus) *hivev1.ClusterDeployment {
	cd := hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
			Annotations: map[string]string{
				"hive.openshift.io/default-AMI": testAMI,
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ClusterName:  testName,
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: corev1.LocalObjectReference{
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
			Installed:             true,
			AdminKubeconfigSecret: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-admin-kubeconfig", testName)},
			ClusterID:             testClusterID,
			InfraID:               testInfraID,
		},
	}

	if syncSetStatus != nil {
		cd.Status.SyncSetStatus = syncSetStatus
	}

	return &cd
}

func testSyncSet() *hivev1.SyncSet {
	return &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testName,
			Namespace: testNamespace,
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources: []runtime.RawExtension{
					{
						Object: testSecret("foo", "bar", "baz"),
					},
				},
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: testName,
				},
			},
		},
	}
}

func testKubeconfig(name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			key: []byte(fmt.Sprintf(kubeconfigTemplate, value)),
		},
	}
	return s
}

func testSecret(name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind: "secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testName,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return s
}

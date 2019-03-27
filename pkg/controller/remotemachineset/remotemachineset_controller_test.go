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

package remotemachineset

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/awsclient"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
)

const (
	testName                 = "foo"
	testNamespace            = "default"
	testClusterID            = "foo-12345-uuid"
	testInfraID              = "foo-12345"
	machineAPINamespace      = "openshift-machine-api"
	metadataName             = "foo-metadata"
	adminKubeconfigSecret    = "foo-admin-kubeconfig"
	adminKubeconfigSecretKey = "kubeconfig"
	adminPasswordSecret      = "foo-admin-creds"
	adminPasswordSecretKey   = "password"
	sshKeySecret             = "foo-ssh-key"
	sshKeySecretKey          = "ssh-publickey"
	testUUID                 = "fakeUUID"
	testAMI                  = "ami-totallyfake"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestRemoteMachineSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)

	// Utility function to list test MachineSets from the fake client
	getRMSL := func(rc client.Client) (*machineapi.MachineSetList, error) {
		rMSL := &machineapi.MachineSetList{}
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
		err := rc.List(context.TODO(), &client.ListOptions{
			Raw: &metav1.ListOptions{TypeMeta: tm},
		}, rMSL)
		if err == nil {
			return rMSL, err
		}
		return nil, err
	}

	tests := []struct {
		name                      string
		localExisting             []runtime.Object
		remoteExisting            []runtime.Object
		expectErr                 bool
		expectedRemoteMachineSets *machineapi.MachineSetList
	}{
		{
			name: "Kubeconfig doesn't exist yet",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{}),
			},
			expectErr: true,
		},
		{
			name: "No-op",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
					},
				}
			}(),
		},
		{
			name: "Update machine set replicas",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 0, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 1),
					},
				}
			}(),
		},
		{
			name: "Create missing machine set",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, []string{"us-east-1a", "us-east-1b", "us-east-1c"}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1c", "worker", false, 1, 0),
					},
				}
			}(),
		},
		{
			name: "Delete extra machine set",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
				testMachineSet("foo-12345-worker-us-east-1d", "worker", true, 1, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-worker-us-east-1a", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1b", "worker", true, 1, 0),
						*testMachineSet("foo-12345-worker-us-east-1c", "worker", true, 1, 0),
					},
				}
			}(),
		},
		{
			name: "Multiple machineset noop",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("alpha", 3, []string{"us-east-1a"}),
					testMachinePool("beta", 3, []string{"us-east-1b"}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 3, 0),
				testMachineSet("foo-12345-beta-us-east-1b", "beta", true, 3, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 3, 0),
						*testMachineSet("foo-12345-beta-us-east-1b", "beta", true, 3, 0),
					},
				}
			}(),
		},
		{
			name: "Update multiple machineset replicas",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("alpha", 3, []string{"us-east-1a"}),
					testMachinePool("beta", 3, []string{"us-east-1b"}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 4, 0),
				testMachineSet("foo-12345-beta-us-east-1b", "beta", true, 4, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 3, 1),
						*testMachineSet("foo-12345-beta-us-east-1b", "beta", true, 3, 1),
					},
				}
			}(),
		},
		{
			name: "Create additional machinepool machinesets",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("alpha", 3, []string{}),
					testMachinePool("beta", 3, []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 1, 0),
				testMachineSet("foo-12345-alpha-us-east-1b", "alpha", true, 1, 0),
				testMachineSet("foo-12345-alpha-us-east-1c", "alpha", true, 1, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 1, 0),
						*testMachineSet("foo-12345-alpha-us-east-1b", "alpha", true, 1, 0),
						*testMachineSet("foo-12345-alpha-us-east-1c", "alpha", true, 1, 0),
						*testMachineSet("foo-12345-beta-us-east-1a", "beta", false, 1, 0),
						*testMachineSet("foo-12345-beta-us-east-1b", "beta", false, 1, 0),
						*testMachineSet("foo-12345-beta-us-east-1c", "beta", false, 1, 0),
					},
				}
			}(),
		},
		{
			name: "Delete additional machinepool machinesets",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("alpha", 3, []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 1, 0),
				testMachineSet("foo-12345-alpha-us-east-1b", "alpha", true, 1, 0),
				testMachineSet("foo-12345-alpha-us-east-1c", "alpha", true, 1, 0),
				testMachineSet("foo-12345-beta-us-east-1a", "alpha", true, 1, 0),
				testMachineSet("foo-12345-beta-us-east-1b", "alpha", true, 1, 0),
				testMachineSet("foo-12345-beta-us-east-1c", "alpha", true, 1, 0),
			},
			expectedRemoteMachineSets: func() *machineapi.MachineSetList {
				return &machineapi.MachineSetList{
					Items: []machineapi.MachineSet{
						*testMachineSet("foo-12345-alpha-us-east-1a", "alpha", true, 1, 0),
						*testMachineSet("foo-12345-alpha-us-east-1b", "alpha", true, 1, 0),
						*testMachineSet("foo-12345-alpha-us-east-1c", "alpha", true, 1, 0),
					},
				}
			}(),
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		machineapi.SchemeBuilder.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.localExisting...)
			remoteFakeClient := fake.NewFakeClient(test.remoteExisting...)

			mockCtrl := gomock.NewController(t)
			mockAWSClient := mockaws.NewMockClient(mockCtrl)
			// Test availability zone retrieval when zones have not been set for machine pool
			mockTestAvailabilityZones(mockAWSClient, "us-east-1", []string{"us-east-1a", "us-east-1b", "us-east-1c"})

			rcd := &ReconcileRemoteMachineSet{
				Client: fakeClient,
				scheme: scheme.Scheme,
				logger: log.WithField("controller", "remotemachineset"),
				remoteClusterAPIClientBuilder: func(string) (client.Client, error) {
					return remoteFakeClient, nil
				},
				awsClientBuilder: func(client.Client, string, string, string) (awsclient.Client, error) {
					return mockAWSClient, nil
				},
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

			if test.expectedRemoteMachineSets != nil {
				rMSL, err := getRMSL(remoteFakeClient)
				if assert.NoError(t, err) {
					for _, eMS := range test.expectedRemoteMachineSets.Items {
						found := false
						for _, rMS := range rMSL.Items {
							if eMS.Name == rMS.Name {
								found = true
								assert.Equal(t, *eMS.Spec.Replicas, *rMS.Spec.Replicas)
								assert.Equal(t, eMS.Generation, rMS.Generation)
								if !reflect.DeepEqual(eMS.ObjectMeta.Labels, rMS.ObjectMeta.Labels) {
									t.Errorf("machineset %v has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Labels, rMS.Labels)
								}
								if !reflect.DeepEqual(eMS.ObjectMeta.Annotations, rMS.ObjectMeta.Annotations) {
									t.Errorf("machineset %v has unexpected annotations:\nexpected: %v\nactual: %v", eMS.Name, eMS.Labels, rMS.Labels)
								}
								if !reflect.DeepEqual(eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels) {
									t.Errorf("machineset %v machinespec has unexpected labels:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Labels, rMS.Spec.Template.Spec.Labels)
								}
								if !reflect.DeepEqual(eMS.Spec.Template.Spec.Taints, rMS.Spec.Template.Spec.Taints) {
									t.Errorf("machineset %v has unexpected taints:\nexpected: %v\nactual: %v", eMS.Name, eMS.Spec.Template.Spec.Taints, rMS.Spec.Template.Spec.Taints)
								}
							}
						}
						if !found {
							t.Errorf("did not find expeceted remote machineset: %v", eMS.Name)
						}
					}
					for _, rMS := range rMSL.Items {
						found := false
						for _, eMS := range test.expectedRemoteMachineSets.Items {
							if rMS.Name == eMS.Name {
								found = true
							}
						}
						if !found {
							t.Errorf("found unexpected remote machineset: %v", rMS.Name)
						}
					}
				}
			}
		})
	}
}

func testMachinePool(name string, replicas int, zones []string) hivev1.MachinePool {
	mpReplicas := int64(replicas)

	testMachinePool := hivev1.MachinePool{
		Name:     name,
		Replicas: &mpReplicas,
		Platform: hivev1.MachinePoolPlatform{
			AWS: &hivev1.AWSMachinePoolPlatform{
				InstanceType: "m4.large",
			},
		},
		Labels: map[string]string{
			"machine.openshift.io/cluster-api-cluster":      testInfraID,
			"machine.openshift.io/cluster-api-machine-role": name,
			"machine.openshift.io/cluster-api-machine-type": name,
		},
		Taints: []corev1.Taint{
			{
				Key:    "foo",
				Value:  "bar",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}

	if len(zones) != 0 {
		testMachinePool.Platform.AWS.Zones = zones
	}

	return testMachinePool
}

func testMachineSet(name string, machineType string, unstompedAnnotation bool, replicas int, generation int) *machineapi.MachineSet {
	msReplicas := int32(replicas)
	ms := machineapi.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": machineType,
				"machine.openshift.io/cluster-api-machine-type": machineType,
			},
			Generation: int64(generation),
		},
		Spec: machineapi.MachineSetSpec{
			Replicas: &msReplicas,
			Template: machineapi.MachineTemplateSpec{
				Spec: machineapi.MachineSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"machine.openshift.io/cluster-api-cluster":      testInfraID,
							"machine.openshift.io/cluster-api-machine-role": machineType,
							"machine.openshift.io/cluster-api-machine-type": machineType,
						},
					},
					Taints: []corev1.Taint{
						{
							Key:    "foo",
							Value:  "bar",
							Effect: corev1.TaintEffectNoSchedule,
						},
					},
				},
			},
		},
	}
	// Add a pre-existing annotation which we will ensure remains in updated machinesets.
	if unstompedAnnotation {
		ms.Annotations = map[string]string{
			"hive.openshift.io/unstomped": "true",
		}
	}
	return &ms
}

func testClusterDeployment(computePools []hivev1.MachinePool) *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testName,
			Namespace:  testNamespace,
			Finalizers: []string{hivev1.FinalizerDeprovision},
			UID:        types.UID("1234"),
			Annotations: map[string]string{
				hiveDefaultAMIAnnotation: testAMI,
			},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ClusterName:  testName,
			ControlPlane: hivev1.MachinePool{},
			Compute:      computePools,
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
}

func mockTestAvailabilityZones(mockAWSClient *mockaws.MockClient, region string, zones []string) {
	availabilityZones := []*ec2.AvailabilityZone{}

	for _, zone := range zones {
		availabilityZones = append(availabilityZones, &ec2.AvailabilityZone{
			RegionName: aws.String(region),
			ZoneName:   aws.String(zone),
		})
	}

	mockAWSClient.EXPECT().DescribeAvailabilityZones(gomock.Any()).Return(
		&ec2.DescribeAvailabilityZonesOutput{
			AvailabilityZones: availabilityZones,
		}, nil).AnyTimes()
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

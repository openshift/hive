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
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/awsclient"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	capiv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
)

const (
	testName                 = "foo"
	testNamespace            = "default"
	clusterAPINamespace      = "openshift-cluster-api"
	metadataName             = "foo-metadata"
	adminKubeconfigSecret    = "foo-admin-kubeconfig"
	adminKubeconfigSecretKey = "kubeconfig"
	adminPasswordSecret      = "foo-admin-creds"
	adminPasswordSecretKey   = "password"
	sshKeySecret             = "foo-ssh-key"
	sshKeySecretKey          = "ssh-publickey"
	pullSecretSecret         = "foo-pull-secret"
	pullSecretSecretKey      = ".dockercfg"
	testUUID                 = "fakeUUID"
	testAMI                  = "ami-totallyfake"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestRemoteMachineSetReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	capiv1.SchemeBuilder.AddToScheme(scheme.Scheme)

	// Utility function to list test MachineSets from the fake client
	getRMSL := func(rc client.Client) (*capiv1.MachineSetList, error) {
		rMSL := &capiv1.MachineSetList{}
		tm := metav1.TypeMeta{}
		tm.SetGroupVersionKind(capiv1.SchemeGroupVersion.WithKind("MachineSet"))
		err := rc.List(context.TODO(), &client.ListOptions{
			Raw: &metav1.ListOptions{TypeMeta: tm},
		}, rMSL)
		if err == nil {
			return rMSL, err
		}
		return nil, err
	}

	// Utility function to get a specific test machine set from the fake client
	getRMS := func(rc client.Client, name string) (*capiv1.MachineSet, error) {
		rMS := &capiv1.MachineSet{}
		rMS.TypeMeta.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cluster.k8s.io",
			Version: "v1alpha1",
			Kind:    "MachineSet",
		})
		err := rc.Get(context.Background(), client.ObjectKey{
			Namespace: remoteClusterAPINamespace,
			Name:      name,
		}, rMS)
		if err != nil {
			return rMS, err
		}
		return rMS, nil
	}

	tests := []struct {
		name           string
		localExisting  []runtime.Object
		remoteExisting []runtime.Object
		expectErr      bool
		validate       func(client.Client, *testing.T)
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
					testMachinePool("worker", 3, testName+"-worker", []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
				testSecret(pullSecretSecret, pullSecretSecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-worker-us-east-1a", 1),
				testMachineSet("foo-worker-us-east-1b", 1),
				testMachineSet("foo-worker-us-east-1c", 1),
			},
			validate: func(rc client.Client, t *testing.T) {
				rMSL, err := getRMSL(rc)
				if err != nil {
					t.Errorf("did not get expected remote machine sets: %v", err)
				}

				for _, ms := range rMSL.Items {
					if ms.Generation != 0 {
						t.Errorf("found unexpected machine set update")
					}
				}
			},
		},
		{
			name: "Update machine set replicas",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, testName+"-worker", []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
				testSecret(pullSecretSecret, pullSecretSecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-worker-us-east-1a", 1),
				testMachineSet("foo-worker-us-east-1b", 1),
				testMachineSet("foo-worker-us-east-1c", 0),
			},
			validate: func(rc client.Client, t *testing.T) {
				// Ensure that replicas have been updated for foo-worker-us-east-1c
				rMS, err := getRMS(rc, "foo-worker-us-east-1c")
				if err != nil {
					t.Errorf("did not get expected remote machine set: %v", err)
				}
				if rMS.Spec.Replicas != nil && *rMS.Spec.Replicas != int32(1) {
					t.Errorf("machine set has unexpected replicas")
				}

				// Ensure that annotations haven't been stomped in updated machine set
				if rMS.Annotations["hive.openshift.io/unstomped"] != "true" {
					t.Errorf("existing hive.openshift.io/unstomped machine set annotation was unexpectedly changed")
				}
			},
		},
		{
			name: "Create missing machine set",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, testName+"-worker", []string{"us-east-1a", "us-east-1b", "us-east-1c"}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
				testSecret(pullSecretSecret, pullSecretSecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-worker-us-east-1a", 1),
				testMachineSet("foo-worker-us-east-1b", 1),
			},
			validate: func(rc client.Client, t *testing.T) {
				rMS, err := getRMS(rc, "foo-worker-us-east-1c")
				if err != nil {
					t.Errorf("did not get expected remote machine set: %v", err)
				}
				if rMS.Spec.Replicas != nil && *rMS.Spec.Replicas != int32(1) {
					t.Errorf("machine set has unexpected replicas")
				}
			},
		},
		{
			name: "Delete extra machine set",
			localExisting: []runtime.Object{
				testClusterDeployment([]hivev1.MachinePool{
					testMachinePool("worker", 3, testName+"-worker", []string{}),
				}),
				testSecret(adminKubeconfigSecret, adminKubeconfigSecretKey, testName),
				testSecret(adminPasswordSecret, adminPasswordSecretKey, testName),
				testSecret(sshKeySecret, sshKeySecretKey, testName),
				testSecret(pullSecretSecret, pullSecretSecretKey, testName),
			},
			remoteExisting: []runtime.Object{
				testMachineSet("foo-worker-us-east-1a", 1),
				testMachineSet("foo-worker-us-east-1b", 1),
				testMachineSet("foo-worker-us-east-1c", 1),
				testMachineSet("foo-worker-us-east-1d", 1),
			},
			validate: func(rc client.Client, t *testing.T) {
				_, err := getRMS(rc, "foo-worker-us-east-1d")
				if err == nil {
					t.Errorf("found unexpected machine set")
				}
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		capiv1.SchemeBuilder.AddToScheme(scheme.Scheme)
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

			if test.validate != nil {
				test.validate(remoteFakeClient, t)
			}

			if err != nil && !test.expectErr {
				t.Errorf("unexpected error: %v", err)
			}

			if err == nil && test.expectErr {
				t.Errorf("expected error but got none")
			}
		})
	}
}

func testMachinePool(name string, replicas int, iamRoleName string, zones []string) hivev1.MachinePool {
	mpReplicas := int64(replicas)

	testMachinePool := hivev1.MachinePool{
		Name:     name,
		Replicas: &mpReplicas,
		Platform: hivev1.MachinePoolPlatform{
			AWS: &hivev1.AWSMachinePoolPlatform{
				AMIID:        testAMI,
				InstanceType: "m4.large",
				IAMRoleName:  iamRoleName,
			},
		},
	}

	if len(zones) != 0 {
		testMachinePool.Platform.AWS.Zones = zones
	}

	return testMachinePool
}

func testMachineSet(name string, replicas int) *capiv1.MachineSet {
	msReplicas := int32(replicas)
	return &capiv1.MachineSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: clusterAPINamespace,
			Annotations: map[string]string{
				"hive.openshift.io/unstomped": "true",
			},
			Labels: map[string]string{
				"sigs.k8s.io/cluster-api-cluster":      testName,
				"sigs.k8s.io/cluster-api-machine-role": "worker",
				"sigs.k8s.io/cluster-api-machine-type": "worker",
			},
		},
		Spec: capiv1.MachineSetSpec{
			Replicas: &msReplicas,
		},
	}
}

func testClusterDeployment(machinePools []hivev1.MachinePool) *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testName,
			Namespace:   testNamespace,
			Finalizers:  []string{hivev1.FinalizerDeprovision},
			UID:         types.UID("1234"),
			Annotations: map[string]string{},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterUUID: testUUID,
			Config: hivev1.InstallConfig{
				Admin: hivev1.Admin{
					Email: "user@example.com",
					Password: corev1.LocalObjectReference{
						Name: adminPasswordSecret,
					},
					SSHKey: &corev1.LocalObjectReference{
						Name: sshKeySecret,
					},
				},
				ClusterID: testName,
				Machines:  machinePools,
				PullSecret: corev1.LocalObjectReference{
					Name: pullSecretSecret,
				},
				Platform: hivev1.Platform{
					AWS: &hivev1.AWSPlatform{
						Region: "us-east-1",
						DefaultMachinePlatform: &hivev1.AWSMachinePoolPlatform{
							AMIID: testAMI,
						},
					},
				},
				Networking: hivev1.Networking{
					Type: hivev1.NetworkTypeOpenshiftSDN,
				},
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
		}, nil)
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

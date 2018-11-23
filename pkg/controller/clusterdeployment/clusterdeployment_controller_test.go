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

package clusterdeployment

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/install"
)

const (
	testName              = "foo"
	installJobName        = "foo-install"
	uninstallJobName      = "foo-uninstall"
	testNamespace         = "default"
	metadataName          = "foo-metadata"
	adminPasswordSecret   = "admin-password"
	sshKeySecret          = "ssh-key"
	pullSecretSecret      = "pull-secret"
	testUUID              = "fakeUUID"
	testAMI               = "ami-totallyfake"
	adminKubeconfigSecret = "foo-admin-kubeconfig"
	adminKubeconfig       = `clusters:
- cluster:
    certificate-authority-data: JUNK
    server: https://foo-api.clusters.example.com:6443
  name: foo
`
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestClusterDeploymentReconcile(t *testing.T) {
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
	getJob := func(c client.Client, name string) *batchv1.Job {
		job := &batchv1.Job{}
		err := c.Get(context.TODO(), client.ObjectKey{Name: name, Namespace: testNamespace}, job)
		if err == nil {
			return job
		}
		return nil
	}
	getInstallJob := func(c client.Client) *batchv1.Job {
		return getJob(c, installJobName)
	}
	getUninstallJob := func(c client.Client) *batchv1.Job {
		return getJob(c, uninstallJobName)
	}

	tests := []struct {
		name      string
		existing  []runtime.Object
		expectErr bool
		validate  func(client.Client, *testing.T)
	}{
		{
			name: "Add finalizer",
			existing: []runtime.Object{
				testClusterDeploymentWithoutFinalizer(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd == nil || !HasFinalizer(cd, hivev1.FinalizerDeprovision) {
					t.Errorf("did not get expected clusterdeployment finalizer")
				}
			},
		},
		{
			name: "Create install job",
			existing: []runtime.Object{
				testClusterDeployment(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				job := getInstallJob(c)
				if job == nil {
					t.Errorf("did not find expected install job")
				}
			},
		},
		{
			name: "Lookup default AMI",
			existing: []runtime.Object{
				testNoDefaultAMIClusterDeployment(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd == nil || cd.Spec.Config.Platform.AWS.DefaultMachinePlatform.AMIID != testAMI {
					t.Errorf("did not get expected default AMI")
				}
			},
		},
		{
			name: "No-op Running install job",
			existing: []runtime.Object{
				testClusterDeployment(),
				testInstallJob(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd == nil || !apiequality.Semantic.DeepEqual(cd, testClusterDeployment()) {
					t.Errorf("got unexpected change in clusterdeployment")
				}
				job := getInstallJob(c)
				if job == nil || !apiequality.Semantic.DeepEqual(job, testInstallJob()) {
					t.Errorf("got unexpected change in install job")
				}
			},
		},
		{
			name: "Parse server URL from admin kubeconfig",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.Installed = true
					cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: adminKubeconfigSecret}
					return cd
				}(),
				testInstallJob(),
				testSecret(adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.Equal(t, cd.Status.APIURL, "https://foo-api.clusters.example.com:6443")
				assert.Equal(t, cd.Status.WebConsoleURL, "https://foo-api.clusters.example.com:6443/console")
			},
		},
		{
			name: "Completed install job",
			existing: []runtime.Object{
				testClusterDeployment(),
				testCompletedInstallJob(),
				testMetadataConfigMap(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd == nil || !cd.Status.Installed {
					t.Errorf("did not get a clusterdeployment with a status of Installed")
					return
				}
			},
		},
		{
			name: "Completed with install job manually deleted",
			existing: []runtime.Object{
				func() *hivev1.ClusterDeployment {
					cd := testClusterDeployment()
					cd.Status.Installed = true
					cd.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{Name: adminKubeconfigSecret}
					return cd
				}(),
				testMetadataConfigMap(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(adminKubeconfigSecret, "kubeconfig", adminKubeconfig),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				assert.True(t, cd.Status.Installed)
				job := getInstallJob(c)
				assert.Nil(t, job)
			},
		},
		{
			name: "Delete cluster deployment",
			existing: []runtime.Object{
				testDeletedClusterDeployment(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
				func() *batchv1.Job {
					job, _, _ := install.GenerateInstallerJob(
						testExpiredClusterDeployment(),
						"fakeserviceaccount",
						"password",
						"sshkey",
						"pullsecret")
					return job
				}(),
			},
			validate: func(c client.Client, t *testing.T) {
				uninstallJob := getUninstallJob(c)
				if uninstallJob == nil {
					t.Errorf("did not find expected uninstall job")
				}

				instJob := getInstallJob(c)
				if instJob != nil {
					t.Errorf("got unexpected install job (expected delete)")
				}
			},
		},
		{
			name: "No-op deleted cluster without finalizer",
			existing: []runtime.Object{
				testDeletedClusterDeploymentWithoutFinalizer(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				uninstallJob := getUninstallJob(c)
				if uninstallJob != nil {
					t.Errorf("got unexpected uninstall job")
				}
			},
		},
		{
			name: "Delete expired cluster deployment",
			existing: []runtime.Object{
				testExpiredClusterDeployment(),
				testSecret(adminPasswordSecret, adminCredsSecretPasswordKey, "password"),
				testSecret(pullSecretSecret, pullSecretKey, "{}"),
				testSecret(sshKeySecret, adminSSHKeySecretKey, "fakesshkey"),
			},
			validate: func(c client.Client, t *testing.T) {
				cd := getCD(c)
				if cd != nil {
					t.Errorf("got unexpected cluster deployment (expected deleted)")
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClient(test.existing...)
			rcd := &ReconcileClusterDeployment{
				Client: fakeClient,
				scheme: scheme.Scheme,
				amiLookupFunc: func(cd *hivev1.ClusterDeployment) (string, error) {
					return testAMI, nil
				},
			}

			_, err := rcd.Reconcile(reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			})

			if test.validate != nil {
				test.validate(fakeClient, t)
			}

			if err != nil && !test.expectErr {
				t.Errorf("Unexpected error: %v", err)
			}
			if err == nil && test.expectErr {
				t.Errorf("Expected error but got none")
			}
		})
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
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
				Machines: []hivev1.MachinePool{},
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
	}
}

func testClusterDeploymentWithoutFinalizer() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Finalizers = []string{}
	return cd
}

func testDeletedClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	return cd
}

func testDeletedClusterDeploymentWithoutFinalizer() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	now := metav1.Now()
	cd.DeletionTimestamp = &now
	cd.Finalizers = []string{}
	return cd
}

func testExpiredClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.CreationTimestamp = metav1.Time{Time: metav1.Now().Add(-60 * time.Minute)}
	cd.Annotations[deleteAfterAnnotation] = "5m"
	return cd
}

func testNoDefaultAMIClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Config.Platform.AWS.DefaultMachinePlatform.AMIID = ""
	return cd
}

func testInstallJob() *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      installJobName,
			Namespace: testNamespace,
		},
		Spec: batchv1.JobSpec{},
	}
	controllerutil.SetControllerReference(testClusterDeployment(), job, scheme.Scheme)
	return job
}

func testCompletedInstallJob() *batchv1.Job {
	job := testInstallJob()
	job.Status.Conditions = []batchv1.JobCondition{
		{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		},
	}
	return job
}

func testMetadataConfigMap() *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	cm.Name = metadataName
	cm.Namespace = testNamespace
	metadataJSON := `{
		"aws": {
			"identifier": {
				"tectonicClusterID": "testFooClusterUUID"
			}
		}
	}`
	cm.Data = map[string]string{"metadata.json": metadataJSON}
	return cm
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

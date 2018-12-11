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

package installmanager

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testClusterName = "test-cluster"
	testNamespace   = "test-namespace"
	testUUID        = "fake-cluster-UUID"

	installerBinary     = "openshift-install"
	terraformBinary     = "terraform"
	fakeInstallerBinary = `#!/bin/sh
echo "Fake Installer"
echo $@
WORKDIR=%s
echo '{"clusterName":"test-cluster","aws":{"region":"us-east-1","identifier":{"openshiftClusterID":"fe953108-f64c-4166-bb8e-20da7665ba00"}}}' > $WORKDIR/metadata.json
mkdir -p $WORKDIR/auth/
echo "fakekubeconfig" > $WORKDIR/auth/kubeconfig
`
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestInstallManager(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name                 string
		existing             []runtime.Object
		failedMetadataSave   bool
		failedKubeconfigSave bool
		failedStatusUpdate   bool
	}{
		{
			name:     "successful install",
			existing: []runtime.Object{testClusterDeployment()},
		},
		{
			name:     "pre-existing secret and configmap",
			existing: []runtime.Object{testClusterDeployment(), testPreexistingConfigMap(), testPreexistingSecret()},
		},
		{
			name:               "failed metadata upload", // a non-fatal error
			existing:           []runtime.Object{testClusterDeployment()},
			failedMetadataSave: true,
		},
		{
			name:               "failed cluster status update", // a non-fatal error
			existing:           []runtime.Object{testClusterDeployment()},
			failedStatusUpdate: true,
		},
		{
			name:                 "failed admin kubeconfig save", // fatal error
			existing:             []runtime.Object{testClusterDeployment()},
			failedKubeconfigSave: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir, err := ioutil.TempDir("", "installmanagertest")
			if !assert.NoError(t, err) {
				t.Fail()
			}
			defer os.RemoveAll(tempDir)
			testLog := log.WithField("test", test.name)
			testLog.WithField("dir", tempDir).Infof("using temporary directory")

			fakeClient := fake.NewFakeClient(test.existing...)

			im := InstallManager{
				LogLevel:      "debug",
				WorkDir:       tempDir,
				InstallConfig: filepath.Join(tempDir, "tempinstallconfig.yml"),
				ClusterName:   testClusterName,
				Namespace:     testNamespace,
				DynamicClient: fakeClient,
			}
			testLog.Debugf("%v", im)
			im.Complete([]string{})

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, installerBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}
			// File contents don't matter for terraform, it won't be called because we're faking the install binary:
			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, terraformBinary), "")) {
				t.Fail()
			}

			// Install config also doesn't get used, we just need a file we can copy:
			if !assert.NoError(t, writeFakeInstallConfig(im.InstallConfig)) {
				t.Fail()
			}

			if test.failedMetadataSave {
				im.uploadClusterMetadata = func(*hivev1.ClusterDeployment, *InstallManager) error {
					return fmt.Errorf("failed to save metadata")
				}
			}

			if test.failedStatusUpdate {
				im.updateClusterDeploymentStatus = func(*hivev1.ClusterDeployment, string, *InstallManager) error {
					return fmt.Errorf("failed to update clusterdeployment status")
				}
			}

			if test.failedKubeconfigSave {
				im.uploadAdminKubeconfig = func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin kubeconfig")
				}
			}

			// We don't want to run the uninstaller, so stub it out
			im.runUninstaller = alwaysSucceedUninstall

			err = im.Run()

			if test.failedKubeconfigSave {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if !test.failedMetadataSave {
				// Ensure we uploaded cluster metadata:
				metadata := &corev1.ConfigMap{}
				err = fakeClient.Get(context.Background(),
					types.NamespacedName{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("%s-metadata", testClusterName),
					},
					metadata)
				if !assert.NoError(t, err) {
					t.Fail()
				}
				_, ok := metadata.Data["metadata.json"]
				assert.True(t, ok)
			}

			if !test.failedKubeconfigSave {
				// Ensure we uploaded admin kubeconfig secret:
				adminKubeconfig := &corev1.Secret{}
				err = fakeClient.Get(context.Background(),
					types.NamespacedName{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("%s-admin-kubeconfig", testClusterName),
					},
					adminKubeconfig)
				if !assert.NoError(t, err) {
					t.Fail()
				}
				_, ok := adminKubeconfig.Data["kubeconfig"]
				assert.True(t, ok)

				if !test.failedStatusUpdate {
					// Ensure we set a status reference to the admin kubeconfig secret:
					cd := &hivev1.ClusterDeployment{}
					err = fakeClient.Get(context.Background(),
						types.NamespacedName{
							Namespace: testNamespace,
							Name:      testClusterName,
						},
						cd)
					if !assert.NoError(t, err) {
						t.Fail()
					}
					assert.Equal(t, adminKubeconfig.Name, cd.Status.AdminKubeconfigSecret.Name)
				}
			}

		})
	}
}

func writeFakeBinary(fileName string, contents string) error {
	data := []byte(contents)
	err := ioutil.WriteFile(fileName, data, 0755)
	return err
}

func writeFakeInstallConfig(fileName string) error {
	// nothing needs to read this so for now just an empty file
	data := []byte("fakefile")
	return ioutil.WriteFile(fileName, data, 0755)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testClusterName,
			Namespace:   testNamespace,
			Finalizers:  []string{hivev1.FinalizerDeprovision},
			UID:         types.UID("1234"),
			Annotations: map[string]string{},
		},
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterUUID: testUUID,
			Config: hivev1.InstallConfig{
				SSHKey: &corev1.LocalObjectReference{
					Name: "ssh-key",
				},
				Machines: []hivev1.MachinePool{},
				PullSecret: corev1.LocalObjectReference{
					Name: "pull-secret",
				},
				Platform: hivev1.Platform{
					AWS: &hivev1.AWSPlatform{
						Region: "us-east-1",
					},
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

func alwaysSucceedUninstall(string, string, string, log.FieldLogger) error {
	log.Debugf("running always successful uninstall")
	return nil
}

func testPreexistingSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName + "-admin-kubeconfig",
			Namespace: testNamespace,
		},
		Data: map[string][]byte{}, // empty test data
	}
}

func testPreexistingConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testClusterName + "-metadata",
			Namespace: testNamespace,
		},
		Data: map[string]string{}, // empty test data
	}
}

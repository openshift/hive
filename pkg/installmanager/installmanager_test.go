package installmanager

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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
	testClusterName      = "test-cluster"
	testNamespace        = "test-namespace"
	sshKeySecretName     = "ssh-key"
	pullSecretSecretName = "pull-secret"
	// testClusterID matches the json blob below:
	testClusterID = "fe953108-f64c-4166-bb8e-20da7665ba00"
	// testInfraID matches the json blob below:
	testInfraID = "test-cluster-fe9531"

	installerBinary     = "openshift-install"
	fakeInstallerBinary = `#!/bin/sh
echo "Fake Installer"
echo $@
WORKDIR=%s
echo '{"clusterName":"test-cluster","infraID":"test-cluster-fe9531","clusterID":"fe953108-f64c-4166-bb8e-20da7665ba00", "aws":{"region":"us-east-1","identifier":[{"kubernetes.io/cluster/dgoodwin-dev":"owned"}]}}' > $WORKDIR/metadata.json
mkdir -p $WORKDIR/auth/
echo "fakekubeconfig" > $WORKDIR/auth/kubeconfig
echo "fakepassword" > $WORKDIR/auth/kubeadmin-password
echo "some fake installer log output" >  /tmp/openshift-install-console.log
`

	fakeSSHAddBinary = `#!/bin/bash
KEY_FILE_PATH=${1}

if [[ ${KEY_FILE_PATH} != "%s" ]]; then
		echo "Parameter not what expected"
		exit 1
fi

exit 0
`
	fakeSSHAgentSockPath = "/path/to/agent/sockfile"
	fakeSSHAgentPID      = "12345"

	alwaysErrorBinary = `#!/bin/sh
exit 1`
)

var (
	fakeSSHAgentBinary = `#!/bin/sh
echo "SSH_AUTH_SOCK=%s; export SSH_AUTH_SOCK;"
echo "SSH_AGENT_PID=%s; export SSH_AGENT_PID;"
echo "echo Agent pid %s;"`
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestCalcSleepSeconds(t *testing.T) {
	assert.Equal(t, 60, calcSleepSeconds(0))
	assert.Equal(t, 120, calcSleepSeconds(1))
	assert.Equal(t, 240, calcSleepSeconds(2))
	assert.Equal(t, 480, calcSleepSeconds(3))
	assert.Equal(t, 86400, calcSleepSeconds(5000493985937))
}

func TestInstallManager(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name                     string
		existing                 []runtime.Object
		failedMetadataSave       bool
		failedKubeconfigSave     bool
		failedStatusUpdate       bool
		failedAdminPasswordSave  bool
		failedUploadInstallerLog bool
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
		{
			name:                    "failed admin username/password save", // fatal error
			existing:                []runtime.Object{testClusterDeployment()},
			failedAdminPasswordSave: true,
		},
		{
			name:                     "failed saving of installer log", // non-fatal
			existing:                 []runtime.Object{testClusterDeployment()},
			failedUploadInstallerLog: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir, err := ioutil.TempDir("", "installmanagertest")
			if !assert.NoError(t, err) {
				t.Fail()
			}
			defer os.RemoveAll(tempDir)
			defer os.Remove(installerConsoleLogFilePath)

			sshKeySecret := testSecret(corev1.SecretTypeOpaque, sshKeySecretName, adminSSHKeySecretKey, "fakesshkey")
			pullSecret := testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecretName, corev1.DockerConfigJsonKey, "{}")
			existing := test.existing
			existing = append(existing, sshKeySecret)
			existing = append(existing, pullSecret)

			fakeClient := fake.NewFakeClient(existing...)

			im := InstallManager{
				LogLevel:              "debug",
				WorkDir:               tempDir,
				ClusterDeploymentName: testClusterName,
				Namespace:             testNamespace,
				DynamicClient:         fakeClient,
			}
			im.Complete([]string{})

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, installerBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}

			if test.failedMetadataSave {
				im.uploadClusterMetadata = func(*hivev1.ClusterDeployment, *InstallManager) error {
					return fmt.Errorf("failed to save metadata")
				}
			}

			if test.failedStatusUpdate {
				im.updateClusterDeploymentStatus = func(*hivev1.ClusterDeployment, string, string, *InstallManager) error {
					return fmt.Errorf("failed to update clusterdeployment status")
				}
			}

			if test.failedKubeconfigSave {
				im.uploadAdminKubeconfig = func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin kubeconfig")
				}
			}

			if test.failedAdminPasswordSave {
				im.uploadAdminPassword = func(*hivev1.ClusterDeployment, *InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin password")
				}
			}

			if test.failedUploadInstallerLog {
				im.uploadInstallerLog = func(*hivev1.ClusterDeployment, *InstallManager, error) error {
					return fmt.Errorf("faiiled to save install log")
				}
			}

			// We don't want to run the uninstaller, so stub it out
			im.runUninstaller = alwaysSucceedUninstall

			err = im.Run()

			if test.failedMetadataSave || test.failedKubeconfigSave || test.failedAdminPasswordSave {
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

				// Ensure we set the cluster ID:
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
				assert.Equal(t, testClusterID, cd.Status.ClusterID)
			}

			if !test.failedMetadataSave && !test.failedKubeconfigSave && !test.failedAdminPasswordSave {
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

			// We don't get to this point if we failed a kubeconfig save:
			if !test.failedMetadataSave && !test.failedAdminPasswordSave && !test.failedKubeconfigSave {
				// Ensure we uploaded admin password secret:
				adminPassword := &corev1.Secret{}
				err = fakeClient.Get(context.Background(),
					types.NamespacedName{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("%s-admin-password", testClusterName),
					},
					adminPassword)
				if !assert.NoError(t, err) {
					t.Fail()
				}

				assert.Equal(t, "kubeadmin", string(adminPassword.Data["username"]))
				assert.Equal(t, "fakepassword", string(adminPassword.Data["password"]))

				if !test.failedStatusUpdate {
					// Ensure we set a status reference to the admin password secret:
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
					assert.Equal(t, adminPassword.Name, cd.Status.AdminPasswordSecret.Name)
				}
			}

			// Install log saving checks
			cm := &corev1.ConfigMap{}
			installLogConfigMapKey := types.NamespacedName{Namespace: testNamespace, Name: fmt.Sprintf("%s-install-log", testClusterName)}
			cmErr := fakeClient.Get(context.Background(), installLogConfigMapKey, cm)
			if !test.failedUploadInstallerLog && !test.failedMetadataSave {
				// Ensure we saved the install output to a configmap
				assert.NoError(t, cmErr, "unexpected error fetching install log configmap")
				assert.Contains(t, cm.Data["log"], "some fake installer log", "did not find expected log contents in configmap")
			} else if test.failedUploadInstallerLog {
				assert.Error(t, cmErr, "expected error when fetching non-existent configmap")
			}

		})
	}
}

func writeFakeBinary(fileName string, contents string) error {
	data := []byte(contents)
	err := ioutil.WriteFile(fileName, data, 0755)
	return err
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
			SSHKey: &corev1.LocalObjectReference{
				Name: "ssh-key",
			},
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: &corev1.LocalObjectReference{
				Name: "pull-secret",
			},
			Platform: hivev1.Platform{
				AWS: &hivev1.AWSPlatform{
					Region: "us-east-1",
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

func testSecret(secretType corev1.SecretType, name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		Type: secretType,
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

func TestCleanupRegex(t *testing.T) {
	tests := []struct {
		name           string
		sourceString   string
		missingStrings []string
	}{
		{
			name: "install log example",
			sourceString: `level=info msg="Consuming \"Worker Ignition Config\" from target directory"
level=info msg="Consuming \"Bootstrap Ignition Config\" from target directory"
level=info msg="Consuming \"Master Ignition Config\" from target directory"
level=info msg="Creating infrastructure resources..."
level=info msg="Waiting up to 30m0s for the Kubernetes API at https://api.test-cluster.example.com:6443..."
level=info msg="API v1.13.4+af45cda up"
level=info msg="Waiting up to 30m0s for the bootstrap-complete event..."
level=info msg="Destroying the bootstrap resources..."
level=info msg="Waiting up to 30m0s for the cluster at https://api.test-cluster.example.com:6443 to initialize..."
level=info msg="Waiting up to 10m0s for the openshift-console route to be created..."
level=info msg="Install complete!"
level=info msg="To access the cluster as the system:admin user when using 'oc', run 'export KUBECONFIG=/output/auth/kubeconfig'"
level=info msg="Access the OpenShift web-console here: https://console-openshift-console.apps.test-cluster.example.com"
level=info msg="Login to the console with user: kubeadmin, password: SomeS-ecret-Passw-ord12-34567"`,
			missingStrings: []string{
				"password",
				"SomeS-ecret-Passw-ord12-34567",
			},
		},
		{
			name: "password at start of line",
			sourceString: `some log line
password at start of line
more log`,
			missingStrings: []string{"password"},
		},
		{
			name: "password in first line",
			sourceString: `first line password more text
second line no magic string`,
			missingStrings: []string{"password"},
		},
		{
			name: "password in last line",
			sourceString: `first line
last line with password in text`,
			missingStrings: []string{"password"},
		},
		{
			name:           "case sensitivity test",
			sourceString:   `abc PaSsWoRd def`,
			missingStrings: []string{"PaSsWoRd"},
		},
	}

	for _, test := range tests {
		cleanedString := cleanupLogOutput(test.sourceString)

		for _, testString := range test.missingStrings {
			assert.False(t, strings.Contains(cleanedString, testString), "testing %v: unexpected string found after cleaning", test.name)
		}
	}

}

func TestGatherLogs(t *testing.T) {
	fakeBootstrapIP := "1.2.3.4"

	tests := []struct {
		name            string
		scriptTemplate  string
		expectedLogData string
		expectedError   bool
	}{
		{
			name:           "cannot execute script",
			scriptTemplate: "not a bash script %s %s",
			expectedError:  true,
		},
		{
			name: "successfully run script file",
			scriptTemplate: `#!/bin/bash
		echo "fake log output %s %s" > log-bundle.tar.gz`,
			expectedLogData: fmt.Sprintf("fake log output %s %s\n", fakeBootstrapIP, fakeBootstrapIP),
		},
		{
			name: "error running script",
			scriptTemplate: `#!/bin/bash
exit 2`,
			expectedError: true,
		},
	}

	for _, test := range tests {
		im := InstallManager{
			LogLevel: "debug",
		}
		assert.NoError(t, im.Complete([]string{}))
		result, err := runGatherScript(fakeBootstrapIP, test.scriptTemplate, &im)
		if test.expectedError {
			assert.Error(t, err, "expected error for test case %s", test.name)
		} else {
			data, err := ioutil.ReadFile(result)
			assert.NoError(t, err, "error reading returned log file data")
			assert.Equal(t, test.expectedLogData, string(data))

			// cleanup saved/copied logfile
			if err := os.RemoveAll(result); err != nil {
				t.Logf("couldn't delete saved log file: %v", err)
			}
		}
	}
}
func TestGetBootstrapIP(t *testing.T) {
	tests := []struct {
		name                      string
		terraformStateFileContent string
		expectedIP                string
		expectedError             bool
	}{
		{
			name:          "terraform file doesn't exist",
			expectedError: true,
		},
		{
			name:                      "get bootstrap IP from terraform file",
			expectedIP:                "1.1.1.1",
			terraformStateFileContent: testTerraformStateFile("1.1.1.1"),
		},
		{
			name:                      "bad json terraform",
			terraformStateFileContent: `{ "notModules": "notResources" }`,
			expectedError:             true,
		},
		{
			name:                      "bootstrap instance gone from terraform file",
			terraformStateFileContent: testTerraformStateFileWithInstanceAndIP("notBootstrap", "1.1.1.1"),
			expectedError:             true,
		},
	}

	for _, test := range tests {
		tempDir, err := ioutil.TempDir("", "installmanagersshtest")
		assert.NoError(t, err, "Errored while setting up temp dir for test")
		defer os.RemoveAll(tempDir)

		im := InstallManager{
			LogLevel: "debug",
			WorkDir:  tempDir,
		}
		assert.NoError(t, im.Complete([]string{}))

		if test.terraformStateFileContent != "" {
			assert.NoError(t, ioutil.WriteFile(filepath.Join(tempDir, "terraform.tfstate"), []byte(test.terraformStateFileContent), 0644),
				"errored creating test terraform state file")
		}

		result, err := getBootstrapIP(&im)

		if test.expectedError {
			assert.Error(t, err, "expected error for test case")
		} else {
			assert.NoError(t, err, "unexpected error for test case")

			assert.Equal(t, test.expectedIP, result, "returned IP not what was expected")
		}

	}
}

func testTerraformStateFile(ip string) string {
	return testTerraformStateFileWithInstanceAndIP("aws_instance.bootstrap", ip)
}

func testTerraformStateFileWithInstanceAndIP(instanceName, ip string) string {
	return fmt.Sprintf(`{
	"modules": [
		{
			"path": [
				"root",
				"bootstrap"
			],
			"resources": {
				"%s": {
					"primary": {
						"attributes": {
							"public_ip": "%s"
						}
					}
				}
			}
		}
	]
}`, instanceName, ip)
}

func TestInstallManagerSSH(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                    string
		existingSSHAgentRunning bool
		expectedEnvVars         map[string]string
		badSSHAgent             bool
		badSSHAdd               bool
		expectedError           bool
	}{
		{
			name:                    "already running SSH agent",
			existingSSHAgentRunning: true,
		},
		{
			name: "no running SSH agent",
			expectedEnvVars: map[string]string{
				"SSH_AUTH_SOCK": fakeSSHAgentSockPath,
				"SSH_AGENT_PID": fakeSSHAgentPID,
			},
		},
		{
			name:          "error on launching SSH agent",
			badSSHAgent:   true,
			expectedError: true,
		},
		{
			name: "error on running ssh-add",
			expectedEnvVars: map[string]string{
				"SSH_AUTH_SOCK": fakeSSHAgentSockPath,
				"SSH_AGENT_PID": fakeSSHAgentPID,
			},
			badSSHAdd:     true,
			expectedError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// clear out env vars for each test loop
			if err := os.Unsetenv("SSH_AUTH_SOCK"); err != nil {
				t.Fatalf("error clearing out existing env var: %v", err)
			}
			if err := os.Unsetenv("SSH_AGENT_PID"); err != nil {
				t.Fatalf("error clearing out existing env var: %v", err)
			}

			// temp dir to hold fake ssh-add and ssh-agent and ssh keys
			testDir, err := ioutil.TempDir("", "installmanagersshfake")
			if err != nil {
				t.Fatalf("error creating directory hold temp ssh items: %v", err)
			}
			defer os.RemoveAll(testDir)

			// create a fake SSH private key file
			sshKeyFile := filepath.Join(testDir, "tempSSHKey")
			if err := ioutil.WriteFile(sshKeyFile, []byte("FAKE SSH KEY CONTENT"), 0600); err != nil {
				t.Fatalf("error creating temporary fake SSH key file: %v", err)
			}

			// create a fake 'ssh-add' binary
			sshAddBinFileContent := fmt.Sprintf(fakeSSHAddBinary, sshKeyFile)
			if test.badSSHAdd {
				sshAddBinFileContent = alwaysErrorBinary
			}
			sshAddBinFile := filepath.Join(testDir, "ssh-add")
			if err := ioutil.WriteFile(sshAddBinFile, []byte(sshAddBinFileContent), 0555); err != nil {
				t.Fatalf("error creating fake ssh-add binary: %v", err)
			}

			// create a fake 'ssh-agent' binary
			sshAgentBinFileContent := fmt.Sprintf(fakeSSHAgentBinary, fakeSSHAgentSockPath, fakeSSHAgentPID, fakeSSHAgentPID)
			if test.badSSHAgent {
				sshAgentBinFileContent = alwaysErrorBinary
			}
			sshAgentBinFile := filepath.Join(testDir, "ssh-agent")
			if err := ioutil.WriteFile(sshAgentBinFile, []byte(sshAgentBinFileContent), 0555); err != nil {
				t.Fatalf("error creating fake ssh-agent binary: %v", err)
			}

			tempDir, err := ioutil.TempDir("", "installmanagersshtestresults")
			if err != nil {
				t.Fatalf("errored while setting up temp dir for test: %v", err)
			}
			defer os.RemoveAll(tempDir)

			im := InstallManager{
				LogLevel: "debug",
				WorkDir:  tempDir,
			}

			if test.existingSSHAgentRunning {
				if err := os.Setenv("SSH_AUTH_SOCK", fakeSSHAgentSockPath); err != nil {
					t.Fatalf("errored setting up fake ssh auth sock env: %v", err)
				}
			}

			im.Complete([]string{})

			// place fake binaries early into path
			origPathEnv := os.Getenv("PATH")
			pathEnv := fmt.Sprintf("%s:%s", testDir, origPathEnv)
			if err := os.Setenv("PATH", pathEnv); err != nil {
				t.Fatalf("error setting PATH (for fake binaries): %v", err)
			}

			cleanup, err := initSSHAgent(sshKeyFile, &im)

			// restore PATH
			if err := os.Setenv("PATH", origPathEnv); err != nil {
				t.Fatalf("error restoring PATH after test: %v", err)
			}

			if test.expectedError {
				assert.Error(t, err, "expected an error while initializing SSH")
			} else {
				assert.NoError(t, err, "unexpected error while testing SSH initialization")
			}

			// check env vars are properly set/cleaned
			if !test.existingSSHAgentRunning {
				for k, v := range test.expectedEnvVars {
					val := os.Getenv(k)
					assert.Equal(t, v, val, "env var %s not expected value", k)
				}

				// cleanup
				cleanup()

				// verify cleanup
				for _, envVar := range test.expectedEnvVars {
					assert.Empty(t, os.Getenv(envVar))
				}
			}

		})
	}
}

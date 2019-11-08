package installmanager

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"

	installertypes "github.com/openshift/installer/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

const (
	testDeploymentName   = "test-deployment"
	testProvisionName    = "test-provision"
	testNamespace        = "test-namespace"
	sshKeySecretName     = "ssh-key"
	pullSecretSecretName = "pull-secret"

	installerBinary     = "openshift-install"
	ocBinary            = "oc"
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

func TestInstallManager(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	tests := []struct {
		name                          string
		existing                      []runtime.Object
		failedMetadataRead            bool
		failedKubeconfigSave          bool
		failedAdminPasswordSave       bool
		failedInstallerLogRead        bool
		failedProvisionUpdate         *int32
		expectKubeconfigSecret        bool
		expectPasswordSecret          bool
		expectProvisionMetadataUpdate bool
		expectProvisionLogUpdate      bool
		expectError                   bool
	}{
		{
			name:                          "successful install",
			existing:                      []runtime.Object{testClusterDeployment(), testClusterProvision()},
			expectKubeconfigSecret:        true,
			expectPasswordSecret:          true,
			expectProvisionMetadataUpdate: true,
			expectProvisionLogUpdate:      true,
		},
		{
			name:               "failed metadata read",
			existing:           []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedMetadataRead: true,
			expectError:        true,
		},
		{
			name:                   "failed cluster provision metadata update",
			existing:               []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedProvisionUpdate:  pointer.Int32Ptr(0),
			expectKubeconfigSecret: true,
			expectPasswordSecret:   true,
			expectError:            true,
		},
		{
			name:                          "failed cluster provision log update", // a non-fatal error
			existing:                      []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedProvisionUpdate:         pointer.Int32Ptr(1),
			expectKubeconfigSecret:        true,
			expectPasswordSecret:          true,
			expectProvisionMetadataUpdate: true,
		},
		{
			name:                 "failed admin kubeconfig save", // fatal error
			existing:             []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedKubeconfigSave: true,
			expectError:          true,
		},
		{
			name:                    "failed admin username/password save", // fatal error
			existing:                []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedAdminPasswordSave: true,
			expectKubeconfigSecret:  true,
			expectError:             true,
		},
		{
			name:                          "failed saving of installer log", // non-fatal
			existing:                      []runtime.Object{testClusterDeployment(), testClusterProvision()},
			failedInstallerLogRead:        true,
			expectKubeconfigSecret:        true,
			expectPasswordSecret:          true,
			expectProvisionMetadataUpdate: true,
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

			// create a fake install-config
			mountedInstallConfigFile := filepath.Join(tempDir, "mounted-install-config.yaml")
			if err := ioutil.WriteFile(mountedInstallConfigFile, []byte("FAKE INSTALL CONFIG"), 0600); err != nil {
				t.Fatalf("error creating temporary fake install-config file: %v", err)
			}

			im := InstallManager{
				LogLevel:               "debug",
				WorkDir:                tempDir,
				ClusterProvisionName:   testProvisionName,
				Namespace:              testNamespace,
				DynamicClient:          fakeClient,
				InstallConfigMountPath: mountedInstallConfigFile,
			}
			im.Complete([]string{})

			im.waitForProvisioningStage = func(*hivev1.ClusterProvision, *InstallManager) error { return nil }

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, installerBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, ocBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}

			if test.failedMetadataRead {
				im.readClusterMetadata = func(*hivev1.ClusterProvision, *InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
					return nil, nil, fmt.Errorf("failed to save metadata")
				}
			}

			if test.failedKubeconfigSave {
				im.uploadAdminKubeconfig = func(*hivev1.ClusterProvision, *InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin kubeconfig")
				}
			}

			if test.failedAdminPasswordSave {
				im.uploadAdminPassword = func(*hivev1.ClusterProvision, *InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin password")
				}
			}

			if test.failedInstallerLogRead {
				im.readInstallerLog = func(*hivev1.ClusterProvision, *InstallManager) (string, error) {
					return "", fmt.Errorf("faiiled to save install log")
				}
			}

			if test.failedProvisionUpdate != nil {
				calls := int32(0)
				im.updateClusterProvision = func(provision *hivev1.ClusterProvision, im *InstallManager, mutation provisionMutation) error {
					callNumber := calls
					calls = calls + 1
					if callNumber == *test.failedProvisionUpdate {
						return fmt.Errorf("failed to update provision")
					}
					return updateClusterProvisionWithRetries(provision, im, mutation)
				}
			}

			// We don't want to run the uninstaller, so stub it out
			im.cleanupFailedProvision = alwaysSucceedCleanupFailedProvision

			err = im.Run()

			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			adminKubeconfig := &corev1.Secret{}
			err = fakeClient.Get(context.Background(),
				types.NamespacedName{
					Namespace: testNamespace,
					Name:      fmt.Sprintf("%s-admin-kubeconfig", testProvisionName),
				},
				adminKubeconfig)
			if test.expectKubeconfigSecret {
				if assert.NoError(t, err) {
					kubeconfig, ok := adminKubeconfig.Data["kubeconfig"]
					if assert.True(t, ok) {
						assert.Equal(t, []byte("fakekubeconfig\n"), kubeconfig, "unexpected kubeconfig")
					}
				}
			} else {
				assert.True(t, apierrors.IsNotFound(err), "unexpected response from getting kubeconfig secret: %v", err)
			}

			adminPassword := &corev1.Secret{}
			err = fakeClient.Get(context.Background(),
				types.NamespacedName{
					Namespace: testNamespace,
					Name:      fmt.Sprintf("%s-admin-password", testProvisionName),
				},
				adminPassword)
			if test.expectPasswordSecret {
				if assert.NoError(t, err) {
					username, ok := adminPassword.Data["username"]
					if assert.True(t, ok) {
						assert.Equal(t, []byte("kubeadmin"), username, "unexpected admin username")
					}
					password, ok := adminPassword.Data["password"]
					if assert.True(t, ok) {
						assert.Equal(t, []byte("fakepassword"), password, "unexpected admin password")
					}
				}
			} else {
				assert.True(t, apierrors.IsNotFound(err), "unexpected response from getting password secret: %v", err)
			}

			provision := &hivev1.ClusterProvision{}
			if err := fakeClient.Get(context.Background(),
				types.NamespacedName{
					Namespace: testNamespace,
					Name:      testProvisionName,
				},
				provision,
			); !assert.NoError(t, err) {
				t.Fail()
			}

			if test.expectProvisionMetadataUpdate {
				assert.NotNil(t, provision.Spec.Metadata, "expected metadata to be set")
				if assert.NotNil(t, provision.Spec.AdminKubeconfigSecret, "expected kubeconfig secret reference to be set") {
					assert.Equal(t, "test-provision-admin-kubeconfig", provision.Spec.AdminKubeconfigSecret.Name, "unexpected name for kubeconfig secret reference")
				}
				if assert.NotNil(t, provision.Spec.AdminPasswordSecret, "expected password secret reference to be set") {
					assert.Equal(t, "test-provision-admin-password", provision.Spec.AdminPasswordSecret.Name, "unexpected name for password secret reference")
				}
			} else {
				assert.Nil(t, provision.Spec.Metadata, "expected metadata to be empty")
				assert.Nil(t, provision.Spec.AdminKubeconfigSecret, "expected kubeconfig secret reference to be empty")
				assert.Nil(t, provision.Spec.AdminPasswordSecret, "expected password secret reference to be empty")
			}

			if test.expectProvisionLogUpdate {
				if assert.NotNil(t, provision.Spec.InstallLog, "expected install log to be set") {
					assert.Equal(t, "some fake installer log output\n", *provision.Spec.InstallLog, "did not find expected contents in saved installer log")
				}
			} else {
				assert.Nil(t, provision.Spec.InstallLog, "expected install log to be empty")
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
			Name:      testDeploymentName,
			Namespace: testNamespace,
		},
	}
}

func testClusterProvision() *hivev1.ClusterProvision {
	return &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testProvisionName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeployment: corev1.LocalObjectReference{
				Name: testDeploymentName,
			},
			Stage: hivev1.ClusterProvisionStageProvisioning,
		},
	}
}

func alwaysSucceedCleanupFailedProvision(client.Client, *hivev1.ClusterDeployment, string, log.FieldLogger) error {
	log.Debugf("running always successful uninstall")
	return nil
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
			scriptTemplate: "not a bash script %s %s %s",
			expectedError:  true,
		},
		{
			name: "successfully run script file",
			scriptTemplate: `#!/bin/bash
		echo "fake log output %s %s" > %s`,
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
		t.Run(test.name, func(t *testing.T) {
			im := InstallManager{
				LogLevel: "debug",
				isGatherLogsEnabled: func() bool {
					return true
				},
			}
			assert.NoError(t, im.Complete([]string{}))
			result, err := im.runGatherScript(fakeBootstrapIP, test.scriptTemplate, "/tmp")
			if test.expectedError {
				assert.Error(t, err, "expected error for test case %s", test.name)
			} else {
				t.Logf("result file: %s", result)
				data, err := ioutil.ReadFile(result)
				assert.NoError(t, err, "error reading returned log file data")
				assert.Equal(t, test.expectedLogData, string(data))

				// cleanup saved/copied logfile
				if err := os.RemoveAll(result); err != nil {
					t.Logf("couldn't delete saved log file: %v", err)
				}
			}
		})
	}
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

			// create a fake install-config
			mountedInstallConfigFile := filepath.Join(testDir, "mounted-install-config.yaml")
			if err := ioutil.WriteFile(mountedInstallConfigFile, []byte("FAKE INSTALL CONFIG"), 0600); err != nil {
				t.Fatalf("error creating temporary fake install-config file: %v", err)
			}

			tempDir, err := ioutil.TempDir("", "installmanagersshtestresults")
			if err != nil {
				t.Fatalf("errored while setting up temp dir for test: %v", err)
			}
			defer os.RemoveAll(tempDir)

			im := InstallManager{
				LogLevel:               "debug",
				WorkDir:                tempDir,
				InstallConfigMountPath: mountedInstallConfigFile,
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

func TestIsBootstrapComplete(t *testing.T) {
	cases := []struct {
		name             string
		errCode          int
		expectedComplete bool
	}{
		{
			name:             "complete",
			errCode:          0,
			expectedComplete: true,
		},
		{
			name:             "not complete",
			errCode:          1,
			expectedComplete: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "TestIsBootstrapComplete")
			if err != nil {
				t.Fatalf("could not create temp dir: %v", err)
			}
			defer os.RemoveAll(dir)
			script := fmt.Sprintf("#!/bin/bash\nexit %d", tc.errCode)
			if err := ioutil.WriteFile(path.Join(dir, "openshift-install"), []byte(script), 0777); err != nil {
				t.Fatalf("could not write openshift-install file: %v", err)
			}
			im := &InstallManager{WorkDir: dir}
			actualComplete := im.isBootstrapComplete()
			assert.Equal(t, tc.expectedComplete, actualComplete, "unexpected bootstrap complete")
		})
	}
}

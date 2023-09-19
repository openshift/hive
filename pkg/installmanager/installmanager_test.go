package installmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awsclient "github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/util/scheme"
	yamlutils "github.com/openshift/hive/pkg/util/yaml"
)

const (
	testDeploymentName   = "test-deployment"
	testProvisionName    = "test-provision"
	testNamespace        = "test-namespace"
	testVPCID            = "testvpc123"
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
		{
			name:        "infraID already set on cluster provision", // fatal error
			existing:    []runtime.Object{testClusterDeployment(), testClusterProvisionWithInfraIDSet()},
			expectError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "installmanagertest")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)
			defer os.Remove(installerConsoleLogFilePath)

			binaryTempDir, err := os.MkdirTemp(tempDir, "bin")
			require.NoError(t, err)

			pullSecret := testSecret(corev1.SecretTypeDockerConfigJson, pullSecretSecretName, corev1.DockerConfigJsonKey, "{}")
			existing := test.existing
			existing = append(existing, pullSecret)

			mocks := setupDefaultMocks(t, existing...)

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			// create a fake install-config
			mountedInstallConfigFile := filepath.Join(tempDir, "mounted-install-config.yaml")
			if err := os.WriteFile(mountedInstallConfigFile, []byte("INSTALL_CONFIG: FAKE"), 0600); err != nil {
				t.Fatalf("error creating temporary fake install-config file: %v", err)
			}

			// create a fake pull secret file
			mountedPullSecretFile := filepath.Join(tempDir, "mounted-pull-secret.json")
			if err := os.WriteFile(mountedPullSecretFile, []byte("{}"), 0600); err != nil {
				t.Fatalf("error creating temporary fake pull secret file: %v", err)
			}

			im := InstallManager{
				LogLevel:               "debug",
				WorkDir:                tempDir,
				ClusterProvisionName:   testProvisionName,
				Namespace:              testNamespace,
				DynamicClient:          mocks.fakeKubeClient,
				InstallConfigMountPath: mountedInstallConfigFile,
				PullSecretMountPath:    mountedPullSecretFile,
				binaryDir:              binaryTempDir,
			}
			im.Complete([]string{})

			im.loadSecrets = func(*InstallManager, *hivev1.ClusterDeployment) {}

			im.waitForProvisioningStage = func(*InstallManager) error { return nil }

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, installerBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}

			if !assert.NoError(t, writeFakeBinary(filepath.Join(tempDir, ocBinary),
				fmt.Sprintf(fakeInstallerBinary, tempDir))) {
				t.Fail()
			}

			if test.failedMetadataRead {
				im.readClusterMetadata = func(*InstallManager) ([]byte, *installertypes.ClusterMetadata, error) {
					return nil, nil, fmt.Errorf("failed to save metadata")
				}
			}

			if test.failedKubeconfigSave {
				im.uploadAdminKubeconfig = func(*InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin kubeconfig")
				}
			}

			if test.failedAdminPasswordSave {
				im.uploadAdminPassword = func(*InstallManager) (*corev1.Secret, error) {
					return nil, fmt.Errorf("failed to save admin password")
				}
			}

			if test.failedInstallerLogRead {
				im.readInstallerLog = func(*InstallManager, bool) (string, error) {
					return "", fmt.Errorf("failed to save install log")
				}
			}

			if test.failedProvisionUpdate != nil {
				calls := int32(0)
				im.updateClusterProvision = func(im *InstallManager, mutation provisionMutation) error {
					callNumber := calls
					calls = calls + 1
					if callNumber == *test.failedProvisionUpdate {
						return fmt.Errorf("failed to update provision")
					}
					return updateClusterProvisionWithRetries(im, mutation)
				}
			}

			// We don't want to run the uninstaller, so stub it out
			im.cleanupFailedProvision = alwaysSucceedCleanupFailedProvision

			// Save the list of actuators so that it can be restored at the end of this test
			im.actuator = &s3LogUploaderActuator{awsClientFn: func(c client.Client, secretName, namespace, region string, logger log.FieldLogger) (awsclient.Client, error) {
				return mocks.mockAWSClient, nil
			}}

			err = im.Run()

			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			adminKubeconfig := &corev1.Secret{}
			err = mocks.fakeKubeClient.Get(context.Background(),
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

					assert.Equal(t, testClusterProvision().Name, adminKubeconfig.Labels[constants.ClusterProvisionNameLabel], "incorrect cluster provision name label")
					assert.Equal(t, constants.SecretTypeKubeConfig, adminKubeconfig.Labels[constants.SecretTypeLabel], "incorrect secret type label")
				}
			} else {
				assert.True(t, apierrors.IsNotFound(err), "unexpected response from getting kubeconfig secret: %v", err)
			}

			adminPassword := &corev1.Secret{}
			err = mocks.fakeKubeClient.Get(context.Background(),
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

					assert.Equal(t, testClusterProvision().Name, adminPassword.Labels[constants.ClusterProvisionNameLabel], "incorrect cluster provision name label")
					assert.Equal(t, constants.SecretTypeKubeAdminCreds, adminPassword.Labels[constants.SecretTypeLabel], "incorrect secret type label")
				}
			} else {
				assert.True(t, apierrors.IsNotFound(err), "unexpected response from getting password secret: %v", err)
			}

			provision := &hivev1.ClusterProvision{}
			if err := mocks.fakeKubeClient.Get(context.Background(),
				types.NamespacedName{
					Namespace: testNamespace,
					Name:      testProvisionName,
				},
				provision,
			); !assert.NoError(t, err) {
				t.Fail()
			}

			if test.expectProvisionMetadataUpdate {
				assert.NotNil(t, provision.Spec.MetadataJSON, "expected metadata to be set")
				if assert.NotNil(t, provision.Spec.AdminKubeconfigSecretRef, "expected kubeconfig secret reference to be set") {
					assert.Equal(t, "test-provision-admin-kubeconfig", provision.Spec.AdminKubeconfigSecretRef.Name, "unexpected name for kubeconfig secret reference")
				}
				if assert.NotNil(t, provision.Spec.AdminPasswordSecretRef, "expected password secret reference to be set") {
					assert.Equal(t, "test-provision-admin-password", provision.Spec.AdminPasswordSecretRef.Name, "unexpected name for password secret reference")
				}
			} else {
				assert.Nil(t, provision.Spec.MetadataJSON, "expected metadata to be empty")
				assert.Nil(t, provision.Spec.AdminKubeconfigSecretRef, "expected kubeconfig secret reference to be empty")
				assert.Nil(t, provision.Spec.AdminPasswordSecretRef, "expected password secret reference to be empty")
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
	err := os.WriteFile(fileName, data, 0755)
	return err
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDeploymentName,
			Namespace: testNamespace,
		},
		Spec: hivev1.ClusterDeploymentSpec{
			Provisioning: &hivev1.Provisioning{},
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
		expectedString string
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
			expectedString: `level=info msg="Consuming \"Worker Ignition Config\" from target directory"
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
REDACTED LINE OF OUTPUT`,
		},
		{
			name: "password at start of line",
			sourceString: `some log line
password at start of line
more log`,
			expectedString: `some log line
REDACTED LINE OF OUTPUT
more log`,
		},
		{
			name: "password in first line",
			sourceString: `first line password more text
second line no magic string`,
			expectedString: `REDACTED LINE OF OUTPUT
second line no magic string`,
		},
		{
			name: "password in last line",
			sourceString: `first line
last line with password in text`,
			expectedString: `first line
REDACTED LINE OF OUTPUT`,
		},
		{
			name:           "case sensitivity test",
			sourceString:   `abc PaSsWoRd def`,
			expectedString: `REDACTED LINE OF OUTPUT`,
		},
		{
			name:         "libvirt ssh connection error in console log",
			sourceString: "Internal error: could not connect to libvirt: virError(Code=38, Domain=7, Message='Cannot recv data: Permission denied, please try again.\\r\\nPermission denied (publickey,gssapi-keyex,gssapi-with-mic,password)",
			// In addition to redacting the line with "password" the
			// escaped carriage returns and newlines are unescaped.
			expectedString: "Internal error: could not connect to libvirt: virError(Code=38, Domain=7, Message='Cannot recv data: Permission denied, please try again.\r\nREDACTED LINE OF OUTPUT",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cleanedString := cleanupLogOutput(test.sourceString)
			assert.Equal(t, test.expectedString, cleanedString,
				"unexpected cleaned string")
		})
	}

}

func TestInstallManagerSSH(t *testing.T) {

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
			testDir, err := os.MkdirTemp("", "installmanagersshfake")
			if err != nil {
				t.Fatalf("error creating directory hold temp ssh items: %v", err)
			}
			defer os.RemoveAll(testDir)

			// create a fake SSH private key file
			sshKeyFile := filepath.Join(testDir, "tempSSHKey")
			if err := os.WriteFile(sshKeyFile, []byte("FAKE SSH KEY CONTENT"), 0600); err != nil {
				t.Fatalf("error creating temporary fake SSH key file: %v", err)
			}

			// create a fake 'ssh-add' binary
			sshAddBinFileContent := fmt.Sprintf(fakeSSHAddBinary, sshKeyFile)
			if test.badSSHAdd {
				sshAddBinFileContent = alwaysErrorBinary
			}
			sshAddBinFile := filepath.Join(testDir, "ssh-add")
			if err := os.WriteFile(sshAddBinFile, []byte(sshAddBinFileContent), 0555); err != nil {
				t.Fatalf("error creating fake ssh-add binary: %v", err)
			}

			// create a fake 'ssh-agent' binary
			sshAgentBinFileContent := fmt.Sprintf(fakeSSHAgentBinary, fakeSSHAgentSockPath, fakeSSHAgentPID, fakeSSHAgentPID)
			if test.badSSHAgent {
				sshAgentBinFileContent = alwaysErrorBinary
			}
			sshAgentBinFile := filepath.Join(testDir, "ssh-agent")
			if err := os.WriteFile(sshAgentBinFile, []byte(sshAgentBinFileContent), 0555); err != nil {
				t.Fatalf("error creating fake ssh-agent binary: %v", err)
			}

			// create a fake install-config
			mountedInstallConfigFile := filepath.Join(testDir, "mounted-install-config.yaml")
			if err := os.WriteFile(mountedInstallConfigFile, []byte("INSTALL_CONFIG: FAKE"), 0600); err != nil {
				t.Fatalf("error creating temporary fake install-config file: %v", err)
			}

			// create a fake pull secret file
			mountedPullSecretFile := filepath.Join(testDir, "mounted-pull-secret.json")
			if err := os.WriteFile(mountedPullSecretFile, []byte("{}"), 0600); err != nil {
				t.Fatalf("error creating temporary fake pull secret file: %v", err)
			}

			tempDir, err := os.MkdirTemp("", "installmanagersshtestresults")
			if err != nil {
				t.Fatalf("errored while setting up temp dir for test: %v", err)
			}
			defer os.RemoveAll(tempDir)

			im := InstallManager{
				LogLevel:               "debug",
				WorkDir:                tempDir,
				InstallConfigMountPath: mountedInstallConfigFile,
				PullSecretMountPath:    mountedPullSecretFile,
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

			cleanup, err := im.initSSHAgent([]string{sshKeyFile})

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
func TestInstallManagerSSHKnownHosts(t *testing.T) {

	tests := []struct {
		name         string
		knownHosts   []string
		expectedFile string
	}{
		{
			name: "single ssh known host",
			knownHosts: []string{
				"192.168.86.100 ecdsa-sha2-nistp256 FOOBAR",
			},
			expectedFile: `192.168.86.100 ecdsa-sha2-nistp256 FOOBAR`,
		},
		{
			name: "multiple ssh known hosts",
			knownHosts: []string{
				"192.168.86.100 ecdsa-sha2-nistp256 FOOBAR",
				"192.168.86.101 ecdsa-sha2-nistp256 FOOBAR2",
				"192.168.86.102 ecdsa-sha2-nistp256 FOOBAR3",
			},
			expectedFile: `192.168.86.100 ecdsa-sha2-nistp256 FOOBAR
192.168.86.101 ecdsa-sha2-nistp256 FOOBAR2
192.168.86.102 ecdsa-sha2-nistp256 FOOBAR3`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tempDir, err := os.MkdirTemp("", "installmanagersshknownhosts")
			require.NoError(t, err, "error creating test tempdir")
			defer os.RemoveAll(tempDir)

			im := InstallManager{
				log: log.WithField("test", test.name),
			}
			err = im.writeSSHKnownHosts(tempDir, test.knownHosts)
			require.NoError(t, err, "error writing ssh known hosts ")

			content, err := os.ReadFile(filepath.Join(tempDir, ".ssh", "known_hosts"))
			require.NoError(t, err, "error reading expected ssh known_hosts file")

			assert.Equal(t, test.expectedFile, string(content), "unexpected known_hosts file contents")
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
			dir, err := os.MkdirTemp("", "TestIsBootstrapComplete")
			if err != nil {
				t.Fatalf("could not create temp dir: %v", err)
			}
			defer os.RemoveAll(dir)
			script := fmt.Sprintf("#!/bin/bash\nexit %d", tc.errCode)
			if err := os.WriteFile(path.Join(dir, "openshift-install"), []byte(script), 0777); err != nil {
				t.Fatalf("could not write openshift-install file: %v", err)
			}
			im := &InstallManager{WorkDir: dir}
			actualComplete := im.isBootstrapComplete()
			assert.Equal(t, tc.expectedComplete, actualComplete, "unexpected bootstrap complete")
		})
	}
}

func Test_pasteInPullSecret(t *testing.T) {
	for _, inputFile := range []string{
		"install-config.yaml",
		"install-config-with-existing-pull-secret.yaml",
	} {
		t.Run(inputFile, func(t *testing.T) {
			icData, err := os.ReadFile(filepath.Join("testdata", inputFile))
			if !assert.NoError(t, err, "unexpected error reading install-config.yaml") {
				return
			}
			expected, err := os.ReadFile(filepath.Join("testdata", "install-config-with-pull-secret.yaml"))
			if !assert.NoError(t, err, "unexpected error reading install-config-with-pull-secret.yaml") {
				return
			}
			actual, err := pasteInPullSecret(icData, filepath.Join("testdata", "pull-secret.json"))
			assert.NoError(t, err, "unexpected error pasting in pull secret")
			assert.Equal(t, string(expected), string(actual), "unexpected InstallConfig with pasted pull secret")
		})
	}
}

func TestPatchWorkerMachineSet(t *testing.T) {
	machineSetYAMLBase := `---
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
spec:
  replicas: 1
  template:
    metadata:
      labels:
        machine.openshift.io/cluster-api-cluster: test-czcpt
        machine.openshift.io/cluster-api-machine-role: worker
        machine.openshift.io/cluster-api-machine-type: worker
        machine.openshift.io/cluster-api-machineset: test-czcpt-worker-us-east-1a
    spec:
      providerSpec:
        value:
          ami:
            id: ami-03d1c2cba04df838c
          apiVersion: awsproviderconfig.openshift.io/v1beta1
          blockDevices:
          - ebs:
              encrypted: true
              volumeSize: 120
              volumeType: gp3
          credentialsSecret:
            name: aws-cloud-credentials
          iamInstanceProfile:
            id: test-czcpt-worker-profile
          instanceType: m4.large
          kind: AWSMachineProviderConfig
          placement:
            availabilityZone: us-east-1a
            region: us-east-1
          securityGroups:
          - filters:
            - name: tag:Name
              values:` // doesn't end in a newline, each test appends to the string

	cases := []struct {
		name           string
		manifestYAML   string
		expectModified bool
		expectErr      bool
	}{
		{
			name: "Patch applies with one security group",
			manifestYAML: machineSetYAMLBase +
				"\n              - test-czcpt-worker-sg",
			expectModified: true,
		},
		{
			name: "Patch applies with more than one security group",
			manifestYAML: machineSetYAMLBase +
				"\n              - an-extra-sg" +
				"\n              - another-sg",
			expectModified: true,
		},
		{
			name:           "Patch applies with no security groups",
			manifestYAML:   machineSetYAMLBase + " []",
			expectModified: true,
		},
		{
			name: "Manifest is not a MachineSet",
			manifestYAML: `---
kind: Potato
spec:
  template:
    metadata:
      labels:
      - "potato.openshift.io/potato-api-potato-type": "yukongold"
`,
			expectModified: false,
		},
		{
			name: "Manifest is not a worker MachineSet",
			manifestYAML: `---
kind: MachineSet
spec:
  template:
    metadata:
      labels:
      - "machine.openshift.io/cluster-api-machine-type": "infra"
`,
			expectModified: false,
		},
		{
			name: "Security group is already configured",
			manifestYAML: machineSetYAMLBase +
				"\n              - test-security-group",
			expectModified: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			pool := &hivev1.MachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testpool",
					Namespace: "testnamespace",
					Annotations: map[string]string{
						constants.ExtraWorkerSecurityGroupAnnotation: "test-security-group",
					},
				},
			}
			logger := log.WithFields(log.Fields{"machinePool": pool.Name})
			modifiedBytes, err := patchWorkerMachineSetManifest([]byte(tc.manifestYAML), pool, testVPCID, logger)
			if tc.expectModified {
				assert.NotNil(t, modifiedBytes, "expected manifest to be modified")
			} else {
				assert.Nil(t, modifiedBytes, "expected manifest to not be modified")
			}
			if tc.expectErr {
				assert.Error(t, err, "expected error patching worker machineset manifests")
			} else {
				assert.NoError(t, err, "unexpected error patching worker machineset manifests")
			}

			scheme := scheme.GetScheme()
			codecFactory := serializer.NewCodecFactory(scheme)
			decoder := codecFactory.UniversalDecoder(machineapi.SchemeGroupVersion)

			if tc.expectModified {
				machineSetObj, _, err := decoder.Decode(*modifiedBytes, nil, nil)
				assert.NoError(t, err, "expected to be able to decode MachineSet yaml")
				machineSet, _ := machineSetObj.(*machineapi.MachineSet)

				awsMachineTemplate := new(machineapi.AWSMachineProviderConfig)
				err = json.Unmarshal(machineSet.Spec.Template.Spec.ProviderSpec.Value.Raw, &awsMachineTemplate)
				assert.NoError(t, err, "expected to be able to decode AWSMachineProviderConfig")

				assert.Contains(t, awsMachineTemplate.SecurityGroups[0].Filters[0].Values, "test-security-group", "expected test-security-group to be configured within Security Group Filters in AWSMachineProviderConfig")
				assert.Equal(t, awsMachineTemplate.SecurityGroups[0].Filters[1].Name, "vpc-id", "expected an vpc-id named filter to be configured within Security Group Filters in AWSMachineProviderConfig")
				assert.Contains(t, awsMachineTemplate.SecurityGroups[0].Filters[1].Values, "testvpc123", "expected testvpc123 to be configured within Security Group Filters in AWSMachineProviderConfig")
			}
		})
	}
}

func Test_patchAzureOverrideCreds(t *testing.T) {
	var clusterInfraConfigBytes []byte = []byte(`---
apiVersion: config.openshift.io/v1
kind: Infrastructure
status:
  infrastructureName: hive-cluster-g7fqb
  platformStatus:
    azure:
      cloudName: AzurePublicCloud
      resourceGroupName: hive-cluster-g7fqb-rg
`)

	cases := []struct {
		name                string
		overrideSecretBytes []byte
		expectModified      bool
		expectErr           bool
	}{
		{
			name: "Patch applies successfully",
			overrideSecretBytes: []byte(`---
apiVersion: v1
data:
  azure_client_id: YWFhCg==
`),
			expectModified: true,
			expectErr:      false,
		},
		//azure_region is base64(centralus)
		{
			name: "Patch applies successfully with region exists and same",
			overrideSecretBytes: []byte(`---
apiVersion: v1
data:
  azure_region: Y2VudHJhbHVz
`),
			expectModified: true,
			expectErr:      false,
		},
		//azure_region is base64(eastus)
		{
			name: "Patch fails due to region exists but different",
			overrideSecretBytes: []byte(`---
apiVersion: v1
data:
  azure_region: ZWFzdHVzCg==
`),
			expectModified: false,
			expectErr:      true,
		},
		{
			name: "Patch fails due to azure_resource_prefix exists",
			overrideSecretBytes: []byte(`---
apiVersion: v1
data:
  azure_resource_prefix: YWFhCg==
`),
			expectModified: false,
			expectErr:      true,
		},
		{
			name: "Patch fails due to azure_resourcegroup exists",
			overrideSecretBytes: []byte(`---
apiVersion: v1
data:
  azure_resourcegroup: YWFhCg==
`),
			expectModified: false,
			expectErr:      true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			modifiedBytes, err := patchAzureOverrideCreds([]byte(tc.overrideSecretBytes), clusterInfraConfigBytes, "centralus")
			if tc.expectModified {
				assert.NotNil(t, modifiedBytes, "expected credential secret to be modified")
			} else {
				assert.Nil(t, modifiedBytes, "expected credential secret to not be modified")
			}
			if tc.expectErr {
				assert.Error(t, err, "expected error patching credential secret")
			} else {
				assert.NoError(t, err, "unexpected error patching credential secret")
			}
			if tc.expectModified {
				c, err := yamlutils.Decode(*modifiedBytes)
				assert.NoError(t, err, "expected to be able to decode patched credential secret")
				isRegionCorrect, _ := yamlutils.Test(c, "/data/azure_region", "Y2VudHJhbHVz")
				assert.True(t, isRegionCorrect, "expected /data/azure_region filled correctly in patched credential secret")
				isPrefixCorrect, _ := yamlutils.Test(c, "/data/azure_resource_prefix", "aGl2ZS1jbHVzdGVyLWc3ZnFi") //base64 for hive-cluster-g7fqb
				assert.True(t, isPrefixCorrect, "expected /data/azure_resource_prefix filled correctly in patched credential secret")
				isGroupCorrect, _ := yamlutils.Test(c, "/data/azure_resourcegroup", "aGl2ZS1jbHVzdGVyLWc3ZnFiLXJn") //base64 for hive-cluster-g7fqb-rg
				assert.True(t, isGroupCorrect, "expected /data/azure_resourcegroup filled correctly in patched credential secret")
			}
		})
	}
}

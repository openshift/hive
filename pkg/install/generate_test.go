package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveassert "github.com/openshift/hive/pkg/test/assert"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	installerImage = "fakeinstallerimage"
	cliImage       = "fakecliimage"
)

const (
	testHttpProxy  = "localhost:3112"
	testHttpsProxy = "localhost:4432"
	testNoProxy    = "example.com,foo.com,bar.org"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestGenerateDeprovision(t *testing.T) {
	dr := testClusterDeprovision()
	job, err := GenerateUninstallerJobForDeprovision(
		dr, "someseviceaccount",
		testHttpProxy, testHttpsProxy, testNoProxy,
		nil,
		map[string]string{}, []corev1.Toleration{})
	assert.Nil(t, err)
	assert.NotNil(t, job)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTP_PROXY", testHttpProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "HTTPS_PROXY", testHttpsProxy)
	hiveassert.AssertAllContainersHaveEnvVar(t, &job.Spec.Template.Spec, "NO_PROXY", testNoProxy)
}

func testClusterDeprovision() *hivev1.ClusterDeprovision {
	return &hivev1.ClusterDeprovision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
		Spec: hivev1.ClusterDeprovisionSpec{
			InfraID:   "test-infra-id",
			ClusterID: "test-cluster-id",
			Platform: hivev1.ClusterDeprovisionPlatform{
				AWS: &hivev1.AWSClusterDeprovision{
					Region: "us-east-1",
					CredentialsSecretRef: &corev1.LocalObjectReference{
						Name: "aws-creds",
					},
				},
			},
		},
	}
}

func TestInstallerPodSpec(t *testing.T) {
	tests := []struct {
		name               string
		clusterDeployment  *hivev1.ClusterDeployment
		provisionName      string
		releaseImage       string
		serviceAccountName string
		pvcName            string
		skipGatherLogs     bool
		extraEnvVars       []corev1.EnvVar
		validate           func(*testing.T, *corev1.PodSpec, error)
	}{
		{
			name: "Test Provision Pod Resource Requests",
			clusterDeployment: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					Provisioning: &hivev1.Provisioning{
						InstallConfigSecretRef: &corev1.LocalObjectReference{Name: "foo"},
					},
				},
				Status: hivev1.ClusterDeploymentStatus{
					InstallerImage: &installerImage,
					CLIImage:       &cliImage,
				},
			},
			provisionName:  "testprovision",
			skipGatherLogs: true,
			extraEnvVars: []corev1.EnvVar{
				{
					Name:  "TESTVAR",
					Value: "TESTVAL",
				},
			},
			validate: func(t *testing.T, actualPodSpec *corev1.PodSpec, actualError error) {
				expectedPodMemoryRequest := resource.MustParse("800Mi")
				actualPodMemoryRequest := actualPodSpec.Containers[0].Resources.Requests[corev1.ResourceMemory]

				assert.Equal(t, expectedPodMemoryRequest, actualPodMemoryRequest, "Incorrect pod memory request")

				for _, container := range append(actualPodSpec.Containers, actualPodSpec.InitContainers...) {
					assert.Contains(t, container.Env, corev1.EnvVar{Name: "TESTVAR", Value: "TESTVAL"})
				}
				assert.NoError(t, actualError)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange

			// Act
			actualPodSpec, actualError := InstallerPodSpec(test.clusterDeployment,
				test.provisionName,
				test.releaseImage,
				test.serviceAccountName,
				testHttpProxy,
				testHttpsProxy,
				testNoProxy,
				test.extraEnvVars)

			// Assert
			test.validate(t, actualPodSpec, actualError)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "HTTP_PROXY", testHttpProxy)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "HTTPS_PROXY", testHttpsProxy)
			hiveassert.AssertAllContainersHaveEnvVar(t, actualPodSpec, "NO_PROXY", testNoProxy)
		})
	}
}

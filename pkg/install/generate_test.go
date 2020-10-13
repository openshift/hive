package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
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

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestGenerateDeprovision(t *testing.T) {
	dr := testClusterDeprovision()
	job, err := GenerateUninstallerJobForDeprovision(dr)
	assert.Nil(t, err)
	assert.NotNil(t, job)
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
					Provisioning: &hivev1.Provisioning{},
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
				actualPodMemoryRequest := actualPodSpec.Containers[2].Resources.Requests[corev1.ResourceMemory]

				assert.Equal(t, expectedPodMemoryRequest, actualPodMemoryRequest, "Incorrect pod memory request")

				for _, container := range actualPodSpec.Containers {
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
				test.pvcName,
				test.skipGatherLogs,
				test.extraEnvVars)

			// Assert
			test.validate(t, actualPodSpec, actualError)
		})
	}
}

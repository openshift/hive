package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testClusterName  = "bar"
	sshKeySecret     = "ssh-key"
	pullSecretSecret = "pull-secret"
	testClusterID    = "cluster-id"
	testInfraID      = "infra-id"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestGenerate(t *testing.T) {
	cd := testClusterDeployment()
	cd.Status.Installed = true
	cd.Spec.PreserveOnDelete = true
	job, err := GenerateUninstallerJobForClusterDeployment(cd, "example.com/fake:latest")
	assert.Nil(t, job)
	assert.NotNil(t, err)
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
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
			ClusterName: testClusterName,
			SSHKey: &corev1.LocalObjectReference{
				Name: sshKeySecret,
			},
			ControlPlane: hivev1.MachinePool{},
			Compute:      []hivev1.MachinePool{},
			PullSecret: &corev1.LocalObjectReference{
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
			ClusterID: testClusterID,
			InfraID:   testInfraID,
		},
	}
	controllerutils.FixupEmptyClusterVersionFields(&cd.Status.ClusterVersionStatus)
	return cd
}

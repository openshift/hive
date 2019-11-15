package install

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

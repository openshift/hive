package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	ospprovider "sigs.k8s.io/cluster-api-provider-openstack/pkg/apis/openstackproviderconfig/v1alpha1"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1osp "github.com/openshift/hive/apis/hive/v1/openstack"
)

// This test is broken! The installer now checks for trunk support by querying the OpenStack service.
func BROKEN__TestOpenStackActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testOSPClusterDeployment(),
			pool:              testOSPPool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker", testInfraID): 3,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			actuator := &OpenStackActuator{
				logger: log.WithField("actuator", "openstackactuator_test"),
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test cast")
				validateOSPMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func validateOSPMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		ospProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*ospprovider.OpenstackProviderSpec)
		if assert.True(t, ok, "failed to convert to openstack provider spec") {
			assert.Equal(t, "Flav", ospProvider.Flavor, "unexpected instance type")
		}
	}
}

func testOSPPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		OpenStack: &hivev1osp.MachinePool{
			Flavor: "Flav",
		},
	}
	return p
}

func testOSPClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		OpenStack: &hivev1osp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "osp-credentials",
			},
			Cloud: "rhos-d",
		},
	}
	return cd
}

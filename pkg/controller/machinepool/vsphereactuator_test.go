package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	vsphereprovider "github.com/openshift/machine-api-operator/pkg/apis/vsphereprovider/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
)

func TestVSphereActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testVSphereClusterDeployment(),
			pool:              testVSpherePool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker", testInfraID): 3,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			actuator := &VSphereActuator{
				logger: log.WithField("actuator", "vsphereactuator_test"),
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test cast")
				validateVSphereMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func validateVSphereMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		vsphereProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*vsphereprovider.VSphereMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to vsphere provider spec") {
			assert.Equal(t, int64(32*1024), vsphereProvider.MemoryMiB, "unexpected MemeoryMiB")
			assert.Equal(t, int32(4), vsphereProvider.NumCPUs, "unexpected NumCPUs")
			assert.Equal(t, int32(4), vsphereProvider.NumCoresPerSocket, "unexpected NumCoresPerSocket")
			assert.Equal(t, int32(512), vsphereProvider.DiskGiB, "unexpected DiskGiB")
		}
	}
}

func testVSpherePool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		VSphere: &hivev1vsphere.MachinePool{
			MemoryMiB:         32 * 1024,
			NumCPUs:           4,
			NumCoresPerSocket: 4,
			OSDisk: hivev1vsphere.OSDisk{
				DiskSizeGB: 512,
			},
		},
	}
	return p
}

func testVSphereClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		VSphere: &hivev1vsphere.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "vsphere-credentials",
			},
		},
	}
	return cd
}

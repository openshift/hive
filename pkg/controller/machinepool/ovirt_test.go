package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	machineapi "github.com/openshift/api/machine/v1beta1"
	ovirtprovider "github.com/openshift/cluster-api-provider-ovirt/pkg/apis/ovirtprovider/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ovirt "github.com/openshift/hive/apis/hive/v1/ovirt"
)

const (
	vmTypeServer string = "server"
	cores        int32  = 4
	sockets      int32  = 1
	memoryMB     int32  = 16384
	sizeGB       int64  = 120
)

func TestOvirtActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
		validatePlatform           func(*testing.T, *ovirtprovider.OvirtMachineProviderSpec)
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testOvirtClusterDeployment(),
			pool:              testOvirtPool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker-0", testInfraID): 3,
			},
			validatePlatform: func(t *testing.T, ovirtProvider *ovirtprovider.OvirtMachineProviderSpec) {
				assert.Equal(t, memoryMB, ovirtProvider.MemoryMB, "unexpected MemeoryMiB")
				assert.Equal(t, cores, ovirtProvider.CPU.Cores, "unexpected number of CPU Cores")
				assert.Equal(t, sockets, ovirtProvider.CPU.Sockets, "unexpected number of CPU Sockets")
				assert.Equal(t, sizeGB, ovirtProvider.OSDisk.SizeGB, "unexpected DiskGiB")
				assert.Equal(t, vmTypeServer, ovirtProvider.VMType, "unexpected VMType")
			},
		},
		{
			name:              "empty pool platform config",
			clusterDeployment: testOvirtClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				m := testOvirtPool()
				m.Spec.Platform.Ovirt = &hivev1ovirt.MachinePool{}
				return m
			}(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker-0", testInfraID): 3,
			},
			validatePlatform: func(t *testing.T, ovirtProvider *ovirtprovider.OvirtMachineProviderSpec) {
				assert.Equal(t, workerUserDataName, ovirtProvider.UserDataSecret.Name, "unexpected UserDataSecret")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			actuator := &OvirtActuator{
				logger: log.WithField("actuator", "ovirtactuator_test"),
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test cast")
				validateOvirtMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.validatePlatform)
			}
		})
	}
}

func validateOvirtMachineSets(t *testing.T,
	mSets []*machineapi.MachineSet,
	expectedMSReplicas map[string]int64,
	validatePlatform func(*testing.T, *ovirtprovider.OvirtMachineProviderSpec)) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		ovirtProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*ovirtprovider.OvirtMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to ovirt provider spec") {
			validatePlatform(t, ovirtProvider)
		}
	}
}

func testOvirtPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		Ovirt: &hivev1ovirt.MachinePool{
			CPU: &hivev1ovirt.CPU{
				Cores:   cores,
				Sockets: sockets,
			},
			MemoryMB: memoryMB,
			OSDisk: &hivev1ovirt.Disk{
				SizeGB: sizeGB,
			},
			VMType: hivev1ovirt.VMType(vmTypeServer),
		},
	}
	return p
}

func testOvirtClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		Ovirt: &hivev1ovirt.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "ovirt-credentials",
			},
		},
	}
	return cd
}

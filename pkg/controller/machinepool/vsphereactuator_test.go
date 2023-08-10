package machinepool

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	machineapi "github.com/openshift/api/machine/v1beta1"
	vsphereutil "github.com/openshift/machine-api-operator/pkg/controller/vsphere"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestVSphereActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		masterMachine              *machineapi.Machine
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate machineset",
			clusterDeployment: testVSphereClusterDeployment(),
			pool:              testVSpherePool(),
			expectedMachineSetReplicas: map[string]int64{
				fmt.Sprintf("%s-worker-0", testInfraID): 3,
			},
			masterMachine: testVSphereMachine("master0", "master"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := scheme.GetScheme()

			actuator, err := NewVSphereActuator(test.masterMachine, scheme, log.WithField("actuator", "vsphereactuator_test"))
			assert.NoError(t, err, "unexpected error creating VSphereActuator")

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

		vsphereProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.VSphereMachineProviderSpec)
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

func testVSphereMachineSpec(machineType string) machineapi.MachineSpec {
	rawVSphereProviderSpec, err := vsphereutil.RawExtensionFromProviderSpec(testVSphereProviderSpec())
	if err != nil {
		log.WithError(err).Fatal("error encoding VSphere machine provider spec")
	}
	return machineapi.MachineSpec{
		ObjectMeta: machineapi.ObjectMeta{
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster":      testInfraID,
				"machine.openshift.io/cluster-api-machine-role": machineType,
				"machine.openshift.io/cluster-api-machine-type": machineType,
			},
		},
		ProviderSpec: machineapi.ProviderSpec{
			Value: rawVSphereProviderSpec,
		},
		Taints: []corev1.Taint{
			{
				Key:    "foo",
				Value:  "bar",
				Effect: corev1.TaintEffectNoSchedule,
			},
		},
	}
}

func testVSphereMachine(name string, machineType string) *machineapi.Machine {
	return &machineapi.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels: map[string]string{
				"machine.openshift.io/cluster-api-cluster": testInfraID,
			},
		},
		Spec: testVSphereMachineSpec(machineType),
	}
}

func testVSphereProviderSpec() *machineapi.VSphereMachineProviderSpec {
	return &machineapi.VSphereMachineProviderSpec{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VSphereMachineProviderSpec",
			APIVersion: machineapi.SchemeGroupVersion.String(),
		},
		NumCPUs: 8,
		DiskGiB: 120,
	}
}

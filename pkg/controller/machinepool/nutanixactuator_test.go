package machinepool

//
//import (
//	"fmt"
//	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
//	testfake "github.com/openshift/hive/pkg/test/fake"
//	vsphereutil "github.com/openshift/machine-api-operator/pkg/controller/vsphere"
//	"testing"
//
//	log "github.com/sirupsen/logrus"
//	"github.com/stretchr/testify/assert"
//	"github.com/stretchr/testify/require"
//
//	corev1 "k8s.io/api/core/v1"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//
//	machineapi "github.com/openshift/api/machine/v1beta1"
//	machineapiv1 "github.com/openshift/api/machine/v1"
//	//nutanixutil "github.com/openshift/machine-api-operator/pkg/controller/nutanix"
//
//	hivev1 "github.com/openshift/hive/apis/hive/v1"
//	hivev1nutanix "github.com/openshift/hive/apis/hive/v1/nutanix"
//	"github.com/openshift/hive/pkg/util/scheme"
//)
//
//func TestNutanixActuator(t *testing.T) {
//	tests := []struct {
//		name                       string
//		clusterDeployment          *hivev1.ClusterDeployment
//		pool                       *hivev1.MachinePool
//		masterMachine              *machineapi.Machine
//		expectedMachineSetReplicas map[string]int64
//		expectedErr                bool
//	}{
//		{
//			name:              "generate machineset",
//			clusterDeployment: testNutanixClusterDeployment(),
//			pool:              testNutanixPool(),
//			expectedMachineSetReplicas: map[string]int64{
//				fmt.Sprintf("%s-worker-0", testInfraID): 3,
//			},
//			masterMachine: testNutanixMachine("master0", "master"),
//		},
//	}
//
//	for _, test := range tests {
//		t.Run(test.name, func(t *testing.T) {
//			scheme := scheme.GetScheme()
//
//			fakeClient := testfake.NewFakeClientBuilder().Build()
//			actuator, err := NewNutanixActuator(fakeClient, scheme, log.WithField("actuator", "nutanixactuator_test"))
//			assert.NoError(t, err, "unexpected error creating NutanixActuator")
//
//			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)
//
//			if test.expectedErr {
//				assert.Error(t, err, "expected error for test case")
//			} else {
//				require.NoError(t, err, "unexpected error for test cast")
//				validateNutanixMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
//			}
//		})
//	}
//}
//
//func validateNutanixMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
//	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")
//
//	for _, ms := range mSets {
//		expectedReplicas, ok := expectedMSReplicas[ms.Name]
//		if assert.True(t, ok, "unexpected machine set") {
//			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
//		}
//
//		nutanixProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapiv1.NutanixMachineProviderConfig)
//		if assert.True(t, ok, "failed to convert to vsphere provider spec") {
//			assert.Equal(t, int64(32*1024), nutanixProvider.MemorySize, "unexpected MemeoryMiB")
//			assert.Equal(t, int32(4), nutanixProvider.VCPUSockets, "unexpected NumCPUs")
//			assert.Equal(t, int32(4), nutanixProvider.VCPUsPerSocket, "unexpected NumCoresPerSocket")
//			assert.Equal(t, int32(512), nutanixProvider.DataDisks, "unexpected DiskGiB")
//			assert.Equal(t, "/vsphere-datacenter/vm/vsphere-folder", vsphereProvider.Workspace.Folder, "unexpected Folder")
//			assert.Equal(t, "/vsphere-datacenter/host/vsphere-cluster/Resources/vsphere-pool", vsphereProvider.Workspace.ResourcePool, "unexpected ResourcePool")
//		}
//	}
//}
//
//func testNutanixPool() *hivev1.MachinePool {
//	p := testMachinePool()
//	p.Spec.Platform = hivev1.MachinePoolPlatform{
//		Nutanix: &hivev1nutanix.MachinePool{
//			MemoryMiB:         32 * 1024,
//			NumCPUs:           4,
//			NumCoresPerSocket: 4,
//			OSDisk: hivev1nutanix.OSDisk{
//				DiskSizeGiB: 512,
//			},
//		},
//	}
//	return p
//}
//
//func testNutanixClusterDeployment() *hivev1.ClusterDeployment {
//	cd := testClusterDeployment()
//	cd.Spec.Platform = hivev1.Platform{
//		VSphere: &hivev1vsphere.Platform{
//			CredentialsSecretRef: corev1.LocalObjectReference{
//				Name: "vsphere-credentials",
//			},
//			Folder: "/vsphere-datacenter/vm/vsphere-folder",
//		},
//	}
//	return cd
//}
//
//func testNutanixMachineSpec(machineType string) machineapi.MachineSpec {
//	rawVSphereProviderSpec, err := vsphereutil.RawExtensionFromProviderSpec(testVSphereProviderSpec())
//	if err != nil {
//		log.WithError(err).Fatal("error encoding VSphere machine provider spec")
//	}
//	return machineapi.MachineSpec{
//		ObjectMeta: machineapi.ObjectMeta{
//			Labels: map[string]string{
//				"machine.openshift.io/cluster-api-cluster":      testInfraID,
//				"machine.openshift.io/cluster-api-machine-role": machineType,
//				"machine.openshift.io/cluster-api-machine-type": machineType,
//			},
//		},
//		ProviderSpec: machineapi.ProviderSpec{
//			Value: rawVSphereProviderSpec,
//		},
//		Taints: []corev1.Taint{
//			{
//				Key:    "foo",
//				Value:  "bar",
//				Effect: corev1.TaintEffectNoSchedule,
//			},
//		},
//	}
//}
//
//func testNutanixMachine(name string, machineType string) *machineapi.Machine {
//	return &machineapi.Machine{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      name,
//			Namespace: machineAPINamespace,
//			Labels: map[string]string{
//				"machine.openshift.io/cluster-api-cluster": testInfraID,
//			},
//		},
//		Spec: testVSphereMachineSpec(machineType),
//	}
//}
//
//func testNutanixProviderSpec() *machineapi.VSphereMachineProviderSpec {
//	return &machineapiv1.NutanixMachineProviderConfig{
//		TypeMeta: metav1.TypeMeta{
//			Kind:       "VSphereMachineProviderSpec",
//			APIVersion: machineapi.SchemeGroupVersion.String(),
//		},
//		NumCPUs: 8,
//		DiskGiB: 120,
//	}
//}

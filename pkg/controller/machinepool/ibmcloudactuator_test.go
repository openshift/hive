package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	ibmcloudprovider "github.com/openshift/machine-api-provider-ibmcloud/pkg/apis/ibmcloudprovider/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	mockibm "github.com/openshift/hive/pkg/ibmclient/mock"
)

const (
	testIBMInstanceType      = "bx2-4x16"
	testEncryptionKey        = "key1234"
	testDedicatedHostName    = "foo"
	testDedicatedHostProfile = "bar"
)

func TestIBMCloudActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		mockIBMClient              func(*mockibm.MockAPI)
		expectedMachineSetReplicas map[string]int32
		expectedErr                bool
	}{
		{
			name:              "generate machinesets for default region zones",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool:              testIBMCloudPool(),
			mockIBMClient: func(client *mockibm.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{"test-region-1", "test-region-2", "test-region-3"})
			},
			expectedMachineSetReplicas: map[string]int32{
				generateIBMCloudMachineSetName("worker", "1"): 1,
				generateIBMCloudMachineSetName("worker", "2"): 1,
				generateIBMCloudMachineSetName("worker", "3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified Zones",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testIBMCloudPool()
				p.Spec.Platform.IBMCloud.Zones = []string{"test-region-A", "test-region-B", "test-region-C"}
				return p
			}(),
			expectedMachineSetReplicas: map[string]int32{
				generateIBMCloudMachineSetName("worker", "A"): 1,
				generateIBMCloudMachineSetName("worker", "B"): 1,
				generateIBMCloudMachineSetName("worker", "C"): 1,
			},
		},
		{
			name:              "no zones returned for specified region",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool:              testIBMCloudPool(),
			mockIBMClient: func(client *mockibm.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{})
			},
			expectedErr: true,
		},
		{
			name:              "generate machinesets with specified BootVolume",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testIBMCloudPool()
				p.Spec.Platform.IBMCloud.BootVolume = &hivev1ibmcloud.BootVolume{
					EncryptionKey: "key1234",
				}
				return p
			}(),
			mockIBMClient: func(client *mockibm.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{"test-region-1", "test-region-2", "test-region-3"})
			},
			expectedMachineSetReplicas: map[string]int32{
				generateIBMCloudMachineSetName("worker", "1"): 1,
				generateIBMCloudMachineSetName("worker", "2"): 1,
				generateIBMCloudMachineSetName("worker", "3"): 1,
			},
		},
		{
			name:              "generate machinesets with specified DedicatedHosts",
			clusterDeployment: testIBMCloudClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testIBMCloudPool()
				p.Spec.Platform.IBMCloud.DedicatedHosts = []hivev1ibmcloud.DedicatedHost{
					{
						Name: testDedicatedHostName,
					},
					{
						Profile: testDedicatedHostProfile,
					},
				}
				return p
			}(),
			mockIBMClient: func(client *mockibm.MockAPI) {
				mockGetVPCZonesForRegion(client, []string{"test-region-1", "test-region-2", "test-region-3"})
			},
			expectedMachineSetReplicas: map[string]int32{
				generateIBMCloudMachineSetName("worker", "1"): 1,
				generateIBMCloudMachineSetName("worker", "2"): 1,
				generateIBMCloudMachineSetName("worker", "3"): 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)

			ibmcloudClient := mockibm.NewMockAPI(mockCtrl)

			if test.mockIBMClient != nil {
				test.mockIBMClient(ibmcloudClient)
			}

			actuator := &IBMCloudActuator{
				logger:    log.WithField("actuator", "ibmcloudactuator_test"),
				ibmClient: ibmcloudClient,
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				require.NoError(t, err, "unexpected error for test case")

				// Ensure the correct number of machinesets were generated
				if assert.Equal(t, len(test.expectedMachineSetReplicas), len(generatedMachineSets), "different number of machine sets generated than expected") {
					for _, ms := range generatedMachineSets {
						expReplicas, ok := test.expectedMachineSetReplicas[ms.Name]
						if assert.True(t, ok, fmt.Sprintf("machine set with name %s not expected", ms.Name)) {
							assert.Equal(t, expReplicas, *ms.Spec.Replicas, "unexpected number of replicas")
						}
					}
				}

				for _, ms := range generatedMachineSets {
					ibmCloudProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*ibmcloudprovider.IBMCloudMachineProviderSpec)
					if assert.True(t, ok, "failed to convert to ibmcloud provider spec") {
						assert.Equal(t, testIBMInstanceType, ibmCloudProvider.Profile, "unexpected InstanceType")
					}
					// Ensure BootVolume disk encryption settings made it to the resulting MachineSet (if specified):
					if bootVolume := test.pool.Spec.Platform.IBMCloud.BootVolume; bootVolume != nil {
						assert.Equal(t, testEncryptionKey, bootVolume.EncryptionKey, "expected BootVolume EncryptionKey")
					}
					// Ensure DedicatedHosts settings made it to the resulting MachineSet (if specified):
					if dedicatedHosts := test.pool.Spec.Platform.IBMCloud.DedicatedHosts; dedicatedHosts != nil {
						assert.Equal(t, testDedicatedHostName, dedicatedHosts[0].Name, "unexpected DedicatedHost Name")
						assert.Equal(t, testDedicatedHostProfile, dedicatedHosts[1].Profile, "unexpected DedicatedHost Profile")
					}
				}
			}
		})
	}
}

func testIBMCloudPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		IBMCloud: &hivev1ibmcloud.MachinePool{
			InstanceType: testIBMInstanceType,
		},
	}
	return p
}

func testIBMCloudClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		IBMCloud: &hivev1ibmcloud.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "ibmcloud-credentials",
			},
			Region: testRegion,
		},
	}
	return cd
}

func generateIBMCloudMachineSetName(leaseChar, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, leaseChar, zone)
}

func mockGetVPCZonesForRegion(ibmClient *mockibm.MockAPI, zones []string) {
	ibmClient.EXPECT().GetVPCZonesForRegion(gomock.Any(), testRegion).Return(zones, nil).Times(1)
}

package machinepool

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"

	machineapi "github.com/openshift/api/machine/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1alibabacloud "github.com/openshift/hive/apis/hive/v1/alibabacloud"
	mockalibabacloud "github.com/openshift/hive/pkg/alibabaclient/mock"
)

const (
	testAlibabaInstanceType       = "ecs.g7.2xlarge"
	testAlibabaSystemDiskCategory = "cloud_efficiency"
	testAlibabaSystemDiskSize     = 480
	testAlibabaImageID            = "test-alibaba-image-id"
)

func TestAlibabaCloudActuator(t *testing.T) {
	tests := []struct {
		name                       string
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		mockAlibabaCloudClient     func(*mockalibabacloud.MockAPI)
		expectedMachineSetReplicas map[string]int32
		expectedErr                bool
	}{
		{
			name:              "generate machinesets for default region zones",
			clusterDeployment: testAlibabaCloudClusterDeployment(),
			pool:              testAlibabaCloudPool(),
			mockAlibabaCloudClient: func(client *mockalibabacloud.MockAPI) {
				mockGetAvailableZonesByInstanceType(client, []string{"test-region-1", "test-region-2", "test-region-3"}, testAlibabaInstanceType)
			},
			expectedMachineSetReplicas: map[string]int32{
				generateAlibabaCloudMachineSetName("worker-test-region", "1"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "2"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified Zones",
			clusterDeployment: testAlibabaCloudClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testAlibabaCloudPool()
				p.Spec.Platform.AlibabaCloud.Zones = []string{"test-region-A", "test-region-B", "test-region-C"}
				return p
			}(),
			expectedMachineSetReplicas: map[string]int32{
				generateAlibabaCloudMachineSetName("worker-test-region", "A"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "B"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "C"): 1,
			},
		},
		{
			name:              "no zones returned for specified region",
			clusterDeployment: testAlibabaCloudClusterDeployment(),
			pool:              testAlibabaCloudPool(),
			mockAlibabaCloudClient: func(client *mockalibabacloud.MockAPI) {
				mockGetAvailableZonesByInstanceType(client, []string{}, testAlibabaInstanceType)
			},
			expectedErr: true,
		},
		{
			name:              "generate machinesets with specified InstanceType, DiskCategory, DiskSize and Image ID",
			clusterDeployment: testAlibabaCloudClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testAlibabaCloudPool()
				p.Spec.Platform.AlibabaCloud.InstanceType = testAlibabaInstanceType
				p.Spec.Platform.AlibabaCloud.SystemDiskCategory = testAlibabaSystemDiskCategory
				p.Spec.Platform.AlibabaCloud.SystemDiskSize = testAlibabaSystemDiskSize
				p.Spec.Platform.AlibabaCloud.ImageID = testAlibabaImageID
				return p
			}(),
			mockAlibabaCloudClient: func(client *mockalibabacloud.MockAPI) {
				mockGetAvailableZonesByInstanceType(client, []string{"test-region-1", "test-region-2", "test-region-3"}, testAlibabaInstanceType)
			},
			expectedMachineSetReplicas: map[string]int32{
				generateAlibabaCloudMachineSetName("worker-test-region", "1"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "2"): 1,
				generateAlibabaCloudMachineSetName("worker-test-region", "3"): 1,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)

			alibabaClient := mockalibabacloud.NewMockAPI(mockCtrl)

			if test.mockAlibabaCloudClient != nil {
				test.mockAlibabaCloudClient(alibabaClient)
			}

			actuator := &AlibabaCloudActuator{
				logger:        log.WithField("actuator", "alibabacloudactuator_test"),
				alibabaClient: alibabaClient,
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
					alibabaCloudProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AlibabaCloudMachineProviderConfig)
					if assert.True(t, ok, "failed to convert to alibaba cloud provider spec") {
						assert.Equal(t, testAlibabaInstanceType, alibabaCloudProvider.InstanceType, "expected instance type")
					}

					// Ensure system disk category settings made it to the resulting MachineSet (if specified):
					if systemDiskCategory := test.pool.Spec.Platform.AlibabaCloud.SystemDiskCategory; systemDiskCategory != "" {
						assert.Equal(t, testAlibabaSystemDiskCategory, alibabaCloudProvider.SystemDisk.Category, "expected system disk category")
					}
					// Ensure system disk size settings made it to the resulting MachineSet (if specified):
					if systemDiskSize := test.pool.Spec.Platform.AlibabaCloud.SystemDiskSize; systemDiskSize != 0 {
						assert.Equal(t, testAlibabaSystemDiskSize, int(alibabaCloudProvider.SystemDisk.Size), "expected system disk size")
					}
					// Ensure image ID settings made it to the resulting MachineSet (if specified):
					if imageID := test.pool.Spec.Platform.AlibabaCloud.ImageID; imageID != "" {
						assert.Equal(t, testAlibabaImageID, alibabaCloudProvider.ImageID, "expected image ID")
					}
				}
			}
		})
	}
}

func testAlibabaCloudPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		AlibabaCloud: &hivev1alibabacloud.MachinePool{
			InstanceType: testAlibabaInstanceType,
		},
	}
	return p
}

func testAlibabaCloudClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		AlibabaCloud: &hivev1alibabacloud.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "alibabacloud-credentials",
			},
			Region: testRegion,
		},
	}
	return cd
}

func generateAlibabaCloudMachineSetName(leaseChar, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, leaseChar, zone)
}

func mockGetAvailableZonesByInstanceType(alibabaClient *mockalibabacloud.MockAPI, zones []string, instanceType string) {
	alibabaClient.EXPECT().GetAvailableZonesByInstanceType(instanceType).Return(zones, nil).Times(1)
}

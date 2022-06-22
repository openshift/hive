package machinepool

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	mockazure "github.com/openshift/hive/pkg/azureclient/mock"
)

func TestAzureActuator(t *testing.T) {
	tests := []struct {
		name                       string
		mockAzureClient            func(*gomock.Controller, *mockazure.MockClient)
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedImage              *machineapi.Image
		expectedErr                bool
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 3,
			},
		},
		{
			name:              "generate machinesets across zones",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified zones",
			clusterDeployment: testAzureClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				pool := testAzurePool()
				pool.Spec.Platform.Azure.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "more replicas than zones",
			clusterDeployment: testAzureClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testAzurePool()
				p.Spec.Replicas = pointer.Int64Ptr(5)
				return p
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 2,
				generateAzureMachineSetName("zone2"): 2,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "more zones than replicas",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3", "zone4", "zone5"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
				generateAzureMachineSetName("zone4"): 0,
				generateAzureMachineSetName("zone5"): 0,
			},
		},
		{
			name:              "list zones returns zero",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedErr: true,
		},
		{
			name:              "V2 image does not exist, don't check if instance can support V2",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/images/foo-12345",
			},
		},
		{
			name:              "default V2 image exists and instance supports V2 images",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListImagesByResourceGroup(mockCtrl, client, []compute.Image{
					testAzureImage(compute.HyperVGenerationTypesV1),
					testAzureImage(compute.HyperVGenerationTypesV2),
				})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/images/foo-12345-gen2",
			},
		},
		{
			name:              "machinepool provides osImage",
			clusterDeployment: testAzureClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				mp := testAzurePool()
				mp.Spec.Platform.Azure.OSImage = &hivev1azure.OSImage{
					Publisher: "testpublisher",
					Offer:     "testoffer",
					SKU:       "testsku",
					Version:   "testversion",
				}
				return mp
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockGetVMCapabilities(mockCtrl, client, "V1,V2")
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			expectedImage: &machineapi.Image{
				Publisher: "testpublisher",
				Offer:     "testoffer",
				SKU:       "testsku",
				Version:   "testversion",
				Type:      "MarketplaceWithPlan",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			aClient := mockazure.NewMockClient(mockCtrl)

			// set up mock expectations
			test.mockAzureClient(mockCtrl, aClient)

			actuator := &AzureActuator{
				client: aClient,
				logger: log.WithField("actuator", "azureactuator"),
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				assert.NoError(t, err, "unexpected error for test case")
				validateAzureMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedImage)
			}
		})
	}
}

func validateAzureMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64, expectedImage *machineapi.Image) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		azureProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AzureMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to azureProviderSpec") {
			assert.Equal(t, testInstanceType, azureProvider.VMSize, "unexpected instance type")
		}
		if expectedImage != nil {
			assert.Equal(t, expectedImage, &azureProvider.Image)
		}
	}
}

func mockListResourceSKUs(mockCtrl *gomock.Controller, client *mockazure.MockClient, zones []string) {
	page := mockazure.NewMockResourceSKUsPage(mockCtrl)
	client.EXPECT().ListResourceSKUs(gomock.Any(), "").Return(page, nil)
	page.EXPECT().NotDone().Return(true)
	page.EXPECT().Values().Return(
		[]compute.ResourceSku{
			{
				Name: pointer.StringPtr(testInstanceType),
				LocationInfo: &[]compute.ResourceSkuLocationInfo{
					{
						Location: pointer.StringPtr(testRegion),
						Zones:    &zones,
					},
				},
			},
		},
	)
}

func mockGetVMCapabilities(mockCtrl *gomock.Controller, client *mockazure.MockClient, hyperVGenerations string) {
	capabilities := map[string]string{
		"HyperVGenerations": hyperVGenerations,
	}
	client.EXPECT().GetVMCapabilities(gomock.Any(), gomock.Any(), gomock.Any()).Return(capabilities, nil)
}

func mockListImagesByResourceGroup(mockCtrl *gomock.Controller, client *mockazure.MockClient, images []compute.Image) {
	resultPage := compute.NewImageListResultPage(compute.ImageListResult{Value: &images}, func(context.Context, compute.ImageListResult) (compute.ImageListResult, error) {
		return compute.ImageListResult{}, nil
	})
	client.EXPECT().ListImagesByResourceGroup(gomock.Any(), gomock.Any()).Return(&resultPage, nil)
}

func generateAzureMachineSetName(zone string) string {
	return fmt.Sprintf("%s-%s-%s%s", testInfraID, testPoolName, testRegion, zone)
}

func testAzurePool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		Azure: &hivev1azure.MachinePool{
			InstanceType: testInstanceType,
			OSDisk: hivev1azure.OSDisk{
				DiskSizeGB: 120,
				DiskType:   hivev1azure.DefaultDiskType,
			},
		},
	}
	return p
}

func testAzureClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		Azure: &hivev1azure.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "azure-credentials",
			},
			Region: testRegion,
		},
	}
	return cd
}

func testAzureImage(hyperVGen compute.HyperVGenerationTypes) compute.Image {
	return compute.Image{
		ImageProperties: &compute.ImageProperties{
			HyperVGeneration: hyperVGen,
		},
	}
}

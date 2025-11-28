package machinepool

import (
	"context"
	"fmt"
	"testing"

	"github.com/openshift/hive/pkg/constants"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	mockazure "github.com/openshift/hive/pkg/azureclient/mock"
	testlogger "github.com/openshift/hive/pkg/test/logger"
)

type providerSpecValidator func(t *testing.T, providerSpec *machineapi.AzureMachineProviderSpec)

func TestAzureActuator(t *testing.T) {
	tests := []struct {
		name                        string
		mockAzureClient             func(*gomock.Controller, *mockazure.MockClient)
		clusterDeployment           *hivev1.ClusterDeployment
		pool                        *hivev1.MachinePool
		expectedMachineSetReplicas  map[string]int64
		expectedImage               *machineapi.Image
		extraProviderSpecValidation providerSpecValidator
		expectedErr                 bool
		expectedLogs                []string
	}{
		// < 4.12
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
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
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
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
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "default network fields",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			extraProviderSpecValidation: func(t *testing.T, providerSpec *machineapi.AzureMachineProviderSpec) {
				assert.Equal(t, testInfraID+"-rg", providerSpec.NetworkResourceGroup, "unexpected ComputeSubnet => Subnet")
				assert.Equal(t, testInfraID+"-worker-subnet", providerSpec.Subnet, "unexpected ComputeSubnet => Subnet")
				assert.Equal(t, testInfraID+"-vnet", providerSpec.Vnet, "unexpected VirtualNetwork => Vnet")
				assert.False(t, providerSpec.AcceleratedNetworking, "expected basic networking")
				assert.Equal(t, providerSpec.PublicLoadBalancer, testInfraID, "expected default (clusterID) public load balancer")
			},
		},
		{
			name:              "set custom network fields",
			clusterDeployment: testAzureClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				pool := testAzurePool()
				pool.Spec.Platform.Azure.NetworkResourceGroupName = "some-rg"
				pool.Spec.Platform.Azure.ComputeSubnet = "some-subnet"
				pool.Spec.Platform.Azure.VirtualNetwork = "some-vnet"
				pool.Spec.Platform.Azure.VMNetworkingType = "Accelerated"
				pool.Spec.Platform.Azure.OutboundType = "UserDefinedRouting"
				return pool
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			extraProviderSpecValidation: func(t *testing.T, providerSpec *machineapi.AzureMachineProviderSpec) {
				assert.Equal(t, "some-rg", providerSpec.NetworkResourceGroup, "unexpected ComputeSubnet => Subnet")
				assert.Equal(t, "some-subnet", providerSpec.Subnet, "unexpected ComputeSubnet => Subnet")
				assert.Equal(t, "some-vnet", providerSpec.Vnet, "unexpected VirtualNetwork => Vnet")
				assert.True(t, providerSpec.AcceleratedNetworking, "expected accelerated networking")
				assert.Equal(t, providerSpec.PublicLoadBalancer, "", "expected empty public load balancer with UserDefinedRouting")
			},
		},
		{
			name:              "more replicas than zones",
			clusterDeployment: testAzureClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				p := testAzurePool()
				p.Spec.Replicas = ptr.To(int64(5))
				return p
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
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
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
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
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName(""): 3, // Non-zoned deployment
			},
			expectedLogs: []string{"No availability zones detected for region. Using non-zoned deployment."},
		},
		{
			name:              "default V1 image exists and instance supports V1 images",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V1 image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345",
			},
		},
		{
			name:              "default V1 and V2 images exist but instance only supports V1 images",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V1 image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345",
			},
		},
		{
			name:              "default V1 and V2 images exist and instance supports V1 and V2 images",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V2 ("-gen2") image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345-gen2",
			},
		},
		{
			name:              "default V1 and V2 images exist but instance only supports V2 images",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V2 ("-gen2") image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345-gen2",
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
				mockGetVMCapabilities(client, "V1,V2")
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
		// >= 4.12
		{
			name:              "generate single machineset for single zone (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 3,
			},
		},
		{
			name:              "generate machinesets across zones (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified zones (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool: func() *hivev1.MachinePool {
				pool := testAzurePool()
				pool.Spec.Platform.Azure.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "more replicas than zones (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool: func() *hivev1.MachinePool {
				p := testAzurePool()
				p.Spec.Replicas = ptr.To(int64(5))
				return p
			}(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 2,
				generateAzureMachineSetName("zone2"): 2,
				generateAzureMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "more zones than replicas (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3", "zone4", "zone5"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
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
			name:              "list zones returns zero (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName(""): 3, // Non-zoned deployment
			},
			expectedLogs: []string{"No availability zones detected for region. Using non-zoned deployment."},
		},
		{
			name:              "default V1 image exists and instance supports V1 images (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V1 image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345",
			},
		},
		{
			name:              "default V1 and V2 images exist but instance only supports V1 images (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V1 image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345",
			},
		},
		{
			name:              "default V1 and V2 images exist and instance supports V1 and V2 images (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V1,V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V2 ("-gen2") image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345-gen2",
			},
		},
		{
			name:              "default V1 and V2 images exist but instance only supports V2 images (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1", "zone2", "zone3"})
				mockGetVMCapabilities(client, "V2")
				mockListImagesByResourceGroup(client, []compute.Image{testAzureImage(compute.HyperVGenerationTypesV1), testAzureImage(compute.HyperVGenerationTypesV2)})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAzureMachineSetName("zone1"): 1,
				generateAzureMachineSetName("zone2"): 1,
				generateAzureMachineSetName("zone3"): 1,
			},
			// V2 ("-gen2") image is chosen for machinepool
			expectedImage: &machineapi.Image{
				ResourceID: "/resourceGroups/foo-12345-rg/providers/Microsoft.Compute/galleries/gallery_foo_12345/images/foo-12345-gen2",
			},
		},
		{
			name:              "machinepool provides osImage (4.12+)",
			clusterDeployment: testAzureClusterDeployment412(),
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
				mockGetVMCapabilities(client, "V1,V2")
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

			aClient := mockazure.NewMockClient(mockCtrl)

			test.mockAzureClient(mockCtrl, aClient)

			logger, hook := testlogger.NewLoggerWithHook()

			actuator := &AzureActuator{
				client: aClient,
				logger: logger.WithField("actuator", "azureactuator"),
			}

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				assert.NoError(t, err, "unexpected error for test case")
				validateAzureMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedImage, test.extraProviderSpecValidation)
			}

			for _, expectedLog := range test.expectedLogs {
				testlogger.AssertHookContainsMessage(t, hook, expectedLog)
			}
		})
	}
}

func validateAzureMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64, expectedImage *machineapi.Image, epsv providerSpecValidator) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		azureProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AzureMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to azureProviderSpec") {
			assert.Equal(t, testInstanceType, azureProvider.VMSize, "unexpected instance type")
			if expectedImage != nil {
				assert.Equal(t, expectedImage, &azureProvider.Image)
			}
			if epsv != nil {
				epsv(t, azureProvider)
			}
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
				Name: ptr.To(testInstanceType),
				LocationInfo: &[]compute.ResourceSkuLocationInfo{
					{
						Location: ptr.To(testRegion),
						Zones:    &zones,
					},
				},
			},
		},
	)
}

func mockGetVMCapabilities(client *mockazure.MockClient, hyperVGenerations string) {
	capabilities := map[string]string{
		"HyperVGenerations": hyperVGenerations,
	}
	client.EXPECT().GetVMCapabilities(gomock.Any(), gomock.Any(), gomock.Any()).Return(capabilities, nil)
}

func mockListImagesByResourceGroup(client *mockazure.MockClient, images []compute.Image) {
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
	cd.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{
		Azure: &hivev1azure.Metadata{
			ResourceGroupName: ptr.To("foo-12345-rg"),
		},
	}
	return cd
}

func testAzureClusterDeployment412() *hivev1.ClusterDeployment {
	cd := testAzureClusterDeployment()
	cd.Labels[constants.VersionLabel] = "4.12.0"
	return cd
}

func testAzureImage(hyperVGen compute.HyperVGenerationTypes) compute.Image {
	return compute.Image{
		ImageProperties: &compute.ImageProperties{
			HyperVGeneration: hyperVGen,
		},
	}
}

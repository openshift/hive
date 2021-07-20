package machinepool

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	azureprovider "sigs.k8s.io/cluster-api-provider-azure/pkg/apis/azureprovider/v1beta1"

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
		expectedErr                bool
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testAzureClusterDeployment(),
			pool:              testAzurePool(),
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {
				mockListResourceSKUs(mockCtrl, client, []string{"zone1"})
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
			mockAzureClient: func(mockCtrl *gomock.Controller, client *mockazure.MockClient) {},
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
			},
			expectedErr: true,
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
				validateAzureMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func validateAzureMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		azureProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*azureprovider.AzureMachineProviderSpec)
		if assert.True(t, ok, "failed to convert to azureProviderSpec") {
			assert.Equal(t, testInstanceType, azureProvider.VMSize, "unexpected instance type")
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

func generateAzureMachineSetName(zone string) string {
	return fmt.Sprintf("%s-%s-%s%s", testInfraID, testPoolName, testRegion, zone)
}

func testAzurePool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		Azure: &hivev1azure.MachinePool{
			InstanceType: testInstanceType,
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

package remotemachineset

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	compute "google.golang.org/api/compute/v1"
	corev1 "k8s.io/api/core/v1"

	gcpprovider "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
)

func TestGCPActuator(t *testing.T) {
	tests := []struct {
		name                       string
		mockGCPClient              func(*mockgcp.MockClient)
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testGCPClusterDeployment(),
			pool:              testGCPPool(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"testImage"}, testInfraID)
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("zone1"): 3,
			},
		},
		{
			name:              "generate machinesets across zones",
			clusterDeployment: testGCPClusterDeployment(),
			pool:              testGCPPool(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"testImage"}, testInfraID)
				mockListComputeZones(client, []string{"zone1", "zone2", "zone3"}, testRegion)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("zone1"): 1,
				generateGCPMachineSetName("zone2"): 1,
				generateGCPMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified zones",
			clusterDeployment: testGCPClusterDeployment(),
			pool: func() *hivev1.MachinePool {
				pool := testGCPPool()
				pool.Spec.Platform.GCP.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"testImage"}, testInfraID)
			},
			expectedMachineSetReplicas: map[string]int64{
				generateGCPMachineSetName("zone1"): 1,
				generateGCPMachineSetName("zone2"): 1,
				generateGCPMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "list images returns zero",
			clusterDeployment: testGCPClusterDeployment(),
			pool:              testGCPPool(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{}, testInfraID)
			},
			expectedErr: true,
		},
		{
			name:              "list images returns more than 1",
			clusterDeployment: testGCPClusterDeployment(),
			pool:              testGCPPool(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"imageA", "imageB"}, testInfraID)
			},
			expectedErr: true,
		},
		{
			name:              "list zones returns zero",
			clusterDeployment: testGCPClusterDeployment(),
			pool:              testGCPPool(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"imageA"}, testInfraID)
				mockListComputeZones(client, []string{}, testRegion)
			},
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			gClient := mockgcp.NewMockClient(mockCtrl)

			// set up mock expectations
			test.mockGCPClient(gClient)

			ga := &GCPActuator{
				client: gClient,
				logger: log.WithField("actuator", "gcpactuator"),
			}

			generatedMachineSets, err := ga.GenerateMachineSets(test.clusterDeployment, test.pool, ga.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateGCPMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
			}
		})
	}
}

func validateGCPMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		gcpProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*gcpprovider.GCPMachineProviderSpec)
		assert.True(t, ok, "failed to convert to gcpProviderSpec")

		assert.Equal(t, testInstanceType, gcpProvider.MachineType, "unexpected instance type")
	}
}

func mockListComputeZones(gClient *mockgcp.MockClient, zones []string, region string) {
	zoneList := &compute.ZoneList{}

	for _, zone := range zones {
		zoneList.Items = append(zoneList.Items,
			&compute.Zone{
				Name: zone,
			})
	}

	filter := gcpclient.ListComputeZonesOptions{
		Filter: fmt.Sprintf("(region eq '.*%s.*') (status eq UP)", region),
	}
	gClient.EXPECT().ListComputeZones(gomock.Eq(filter)).Return(
		zoneList, nil,
	)
}

func mockListComputeImage(gClient *mockgcp.MockClient, images []string, infraID string) {
	computeImages := &compute.ImageList{}
	for _, image := range images {
		computeImages.Items = append(computeImages.Items,
			&compute.Image{
				Name: image,
			})
	}

	filter := gcpclient.ListComputeImagesOptions{
		Filter: fmt.Sprintf("name eq \"%s-.*\"", infraID),
	}
	gClient.EXPECT().ListComputeImages(gomock.Eq(filter)).Return(
		computeImages, nil,
	)
}

func generateGCPMachineSetName(zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, testPoolName[:1], zone)
}

func testGCPPool() *hivev1.MachinePool {
	p := testMachinePool()
	p.Spec.Platform = hivev1.MachinePoolPlatform{
		GCP: &hivev1gcp.MachinePool{
			InstanceType: testInstanceType,
		},
	}
	return p
}

func testGCPClusterDeployment() *hivev1.ClusterDeployment {
	cd := testClusterDeployment()
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: "gcp-credentials",
			},
			Region: testRegion,
		},
	}
	return cd
}

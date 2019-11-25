package syncmachineset

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	compute "google.golang.org/api/compute/v1"

	gcpprovider "github.com/openshift/cluster-api-provider-gcp/pkg/apis/gcpprovider/v1beta1"
	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
	gcpclient "github.com/openshift/hive/pkg/gcpclient"
	mockgcp "github.com/openshift/hive/pkg/gcpclient/mock"
	"github.com/openshift/hive/pkg/install"
)

const (
	testClusterName = "testCluster"
	testInfraID     = "testCluster-rando"
	testRegion      = "us-east1"
)

type machineSetInfo struct {
	name         string
	replicas     int64
	instanceType string
}

func TestGCPAcuator(t *testing.T) {
	tests := []struct {
		name                string
		mockGCPClient       func(*mockgcp.MockClient)
		clusterDeployment   *hivev1.ClusterDeployment
		expectedMachineSets []machineSetInfo
		expectedErr         bool
	}{
		{
			name: "generate single machineset for single zone",
			clusterDeployment: testCDForMachineSets([]machineSetInfo{
				{
					name:         "poolA",
					replicas:     3,
					instanceType: "testInstance",
				},
			}),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"testImage"}, testInfraID)
				mockListComputeZones(client, []string{"zone1"}, testRegion)
			},
			expectedMachineSets: []machineSetInfo{
				{
					name:         generateMachineSetName("poolA", "zone1"),
					replicas:     3,
					instanceType: "testInstance",
				},
			},
		},
		{
			name: "generate machinesets across zones",
			clusterDeployment: testCDForMachineSets([]machineSetInfo{
				{
					name:         "poolA",
					replicas:     3,
					instanceType: "testInstance",
				},
			}),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"testImage"}, testInfraID)
				mockListComputeZones(client, []string{"zone1", "zone2", "zone3"}, testRegion)
			},
			expectedMachineSets: []machineSetInfo{
				{
					name:         generateMachineSetName("poolA", "zone1"),
					replicas:     1,
					instanceType: "testInstance",
				},
				{
					name:         generateMachineSetName("poolA", "zone2"),
					replicas:     1,
					instanceType: "testInstance",
				},
				{
					name:         generateMachineSetName("poolA", "zone3"),
					replicas:     1,
					instanceType: "testInstance",
				},
			},
		},
		{
			name:              "list images returns zero",
			clusterDeployment: testCDForMachineSets([]machineSetInfo{}),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{}, testInfraID)
			},
			expectedErr: true,
		},
		{
			name:              "list images returns more than 1",
			clusterDeployment: testCDForMachineSets([]machineSetInfo{}),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"imageA", "imageB"}, testInfraID)
			},
			expectedErr: true,
		},
		{
			name: "list zones returns zero",
			clusterDeployment: testCDForMachineSets([]machineSetInfo{
				{
					name:         "poolA",
					replicas:     3,
					instanceType: "testInstance",
				},
			}),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"imageA"}, testInfraID)
				mockListComputeZones(client, []string{}, testRegion)
			},
			expectedErr: true,
		},
		{
			name: "installer generate machinesets errors",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testCDForMachineSets([]machineSetInfo{
					{
						name:         "poolA",
						replicas:     3,
						instanceType: "testInstance",
					},
				})
				cd.Spec.Platform.AWS = &hivev1aws.Platform{}
				return cd
			}(),
			mockGCPClient: func(client *mockgcp.MockClient) {
				mockListComputeImage(client, []string{"imageA"}, testInfraID)
				mockListComputeZones(client, []string{"zoneA"}, testRegion)
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

			ic, err := install.GenerateInstallConfig(test.clusterDeployment, "FAKESSHKEY", "{}", false)
			assert.NoError(t, err, "unexpected error generating install config for test case")

			generatedMachineSets, err := ga.GenerateMachineSets(test.clusterDeployment, ic, ga.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateMachineSets(t, generatedMachineSets, test.expectedMachineSets)
			}
		})
	}
}

func validateMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMS []machineSetInfo) {
	assert.Equal(t, len(expectedMS), len(mSets), "different number of machine sets generated than expected")

	for i, expected := range expectedMS {
		assert.Equal(t, expected.name, mSets[i].Name, "did not find expected machineset: %s", expected.name)

		assert.Equal(t, expected.replicas, int64(*mSets[i].Spec.Replicas), "replica mismatch")

		gcpProvider, ok := mSets[i].Spec.Template.Spec.ProviderSpec.Value.Object.(*gcpprovider.GCPMachineProviderSpec)
		assert.True(t, ok, "failed to convert to gcpProviderSpec")

		assert.Equal(t, expected.instanceType, gcpProvider.MachineType, "unexpected instance type")
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

func testCDForMachineSets(machineSets []machineSetInfo) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: testClusterName,
			Platform: hivev1.Platform{
				GCP: &hivev1gcp.Platform{
					Region: testRegion,
				},
			},
		},
		Status: hivev1.ClusterDeploymentStatus{
			InfraID: testInfraID,
		},
	}

	for _, ms := range machineSets {
		mp := testMachinePool(ms.name, ms.replicas, ms.instanceType)
		cd.Spec.Compute = append(cd.Spec.Compute, mp)
	}

	return cd
}

func generateMachineSetName(poolName, zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, poolName[:1], zone)
}

func testMachinePool(name string, replicas int64, instanceType string) hivev1.MachinePool {
	return hivev1.MachinePool{
		Name:     name,
		Replicas: &replicas,
		Platform: hivev1.MachinePoolPlatform{
			GCP: &hivev1gcp.MachinePool{
				InstanceType: instanceType,
			},
		},
	}
}

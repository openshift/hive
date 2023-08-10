package hibernation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/IBM/vpc-go-sdk/vpcv1"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	"github.com/openshift/hive/pkg/ibmclient"
	mockibmclient "github.com/openshift/hive/pkg/ibmclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestIBMCloudCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.IBMCloud = &hivev1ibmcloud.Platform{}
	}).Build()
	actuator := ibmCloudActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestIBMCloudStopAndStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		instances   map[string]int
		setupClient func(*testing.T, *mockibmclient.MockAPI)
		expectErr   bool
	}{
		{
			name:      "stop no running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deleting": 2, "stopping": 2, "stopped": 1},
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deleting": 5, "running": 2},
			setupClient: func(t *testing.T, c *mockibmclient.MockAPI) {
				c.EXPECT().StopInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Do(
					func(ctx context.Context, instances []vpcv1.Instance, region string) {
						assert.Equal(t, 2, len(instances), "unexpected number of instances provided to StopInstances")
						for _, i := range instances {
							assert.Equal(t, *i.Status, "running")
						}
					},
				).Return(nil)
			},
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deleting": 5, "stopping": 3, "stopped": 4, "pending": 7, "running": 3},
			setupClient: func(t *testing.T, c *mockibmclient.MockAPI) {
				c.EXPECT().StopInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Do(
					func(ctx context.Context, instances []vpcv1.Instance, region string) {
						assert.Equal(t, 10, len(instances), "unexpected number of instances provided to StopInstances")
						for _, i := range instances {
							assert.True(t, *i.Status == "pending" || *i.Status == "running")
						}
					},
				).Return(nil)
			},
		},
		{
			name:      "start no stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"pending": 4, "running": 3},
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"stopped": 3, "running": 4},
			setupClient: func(t *testing.T, c *mockibmclient.MockAPI) {
				c.EXPECT().StartInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Do(
					func(ctx context.Context, instances []vpcv1.Instance, region string) {
						assert.Equal(t, 3, len(instances), "unexpected number of instances provided to StartInstances")
						for _, i := range instances {
							assert.True(t, *i.Status == "stopped")
						}
					},
				).Return(nil)
			},
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"stopped": 3, "stopping": 1, "deleting": 7},
			setupClient: func(t *testing.T, c *mockibmclient.MockAPI) {
				c.EXPECT().StartInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Do(
					func(ctx context.Context, instances []vpcv1.Instance, region string) {
						assert.Equal(t, 4, len(instances), "unexpected number of instances provided to StartInstances")
						for _, i := range instances {
							assert.True(t, *i.Status == "stopped" || *i.Status == "stopping")
						}
					},
				).Return(nil)
			},
		},
		{
			name:      "unable to GetVPCInstances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deleting": 5, "running": 2},
			setupClient: func(t *testing.T, c *mockibmclient.MockAPI) {
				c.EXPECT().GetVPCInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("cannot list instances"))
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ibmCloudClient := mockibmclient.NewMockAPI(ctrl)
			if !test.expectErr {
				setupIBMCloudClientInstances(ibmCloudClient, test.instances)
			}
			if test.setupClient != nil {
				test.setupClient(t, ibmCloudClient)
			}
			actuator := testIBMCloudActuator(ibmCloudClient)
			var err error
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(testIBMCloudClusterDeployment(), nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(testIBMCloudClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			if test.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}

}

func TestIBMCloudMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name              string
		testFunc          string
		expectedRemaining []string
		expectedResult    bool
		instances         map[string]int
		setupClient       func(*testing.T, *mockibmclient.MockAPI)
	}{
		{
			name:           "Stopped - All machines stopped or terminated",
			testFunc:       "MachinesStopped",
			expectedResult: true,
			instances:      map[string]int{"deleting": 3, "stopped": 2},
		},
		{
			name:              "Stopped - Some machines pending",
			testFunc:          "MachinesStopped",
			expectedResult:    false,
			expectedRemaining: []string{"pending-0", "pending-1"},
			instances:         map[string]int{"deleting": 3, "stopped": 2, "pending": 2},
		},
		{
			name:              "Stopped - machines running",
			testFunc:          "MachinesStopped",
			expectedResult:    false,
			expectedRemaining: []string{"running-0", "running-1", "running-2"},
			instances:         map[string]int{"running": 3, "deleting": 2},
		},
		{
			name:              "Running - All machines running",
			testFunc:          "MachinesRunning",
			expectedResult:    true,
			expectedRemaining: []string{},
			instances:         map[string]int{"running": 3},
		},
		{
			name:              "Running - Some machines pending",
			testFunc:          "MachinesRunning",
			expectedResult:    false,
			expectedRemaining: []string{"pending-0"},
			instances:         map[string]int{"running": 3, "pending": 1},
		},
		{
			name:              "Running - Some machines stopped or shutting-down",
			testFunc:          "MachinesRunning",
			expectedResult:    false,
			expectedRemaining: []string{"stopping-0", "stopping-1", "stopping-2", "stopped-0"},
			instances:         map[string]int{"running": 3, "deleting": 2, "stopped": 1, "stopping": 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			ibmCloudClient := mockibmclient.NewMockAPI(ctrl)
			setupIBMCloudClientInstances(ibmCloudClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, ibmCloudClient)
			}
			actuator := testIBMCloudActuator(ibmCloudClient)
			var err error
			var result bool
			var remaining []string
			switch test.testFunc {
			case "MachinesStopped":
				result, remaining, err = actuator.MachinesStopped(testIBMCloudClusterDeployment(), nil, log.New())
			case "MachinesRunning":
				result, remaining, err = actuator.MachinesRunning(testIBMCloudClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			require.Nil(t, err)
			assert.Equal(t, test.expectedResult, result)
			if len(test.expectedRemaining) > 0 {
				sort.Strings(test.expectedRemaining)
				sort.Strings(remaining)
				assert.Equal(t, test.expectedRemaining, remaining)
			}
		})
	}
}

func testIBMCloudActuator(ibmCloudClient ibmclient.API) *ibmCloudActuator {
	return &ibmCloudActuator{
		ibmCloudClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (ibmclient.API, error) {
			return ibmCloudClient, nil
		},
	}
}

func setupIBMCloudClientInstances(ibmCloudClient *mockibmclient.MockAPI, statuses map[string]int) {
	instances := []vpcv1.Instance{}
	for status, count := range statuses {
		for i := 0; i < count; i++ {
			instances = append(instances, vpcv1.Instance{
				Name:   pointer.String(fmt.Sprintf("%s-%d", status, i)),
				ID:     pointer.String(fmt.Sprintf("%s-%d", status, i)),
				Status: pointer.String(status),
			})
		}
	}
	ibmCloudClient.EXPECT().GetVPCInstances(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(instances, nil)
}

func testIBMCloudClusterDeployment() *hivev1.ClusterDeployment {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder("testns", "testibmcluster", scheme)
	return cdBuilder.Build(
		testcd.WithIBMCloudPlatform(&hivev1ibmcloud.Platform{Region: "us-south"}),
		testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: "testibmcluster-foobarbaz"}),
	)
}

package hibernation

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	compute "google.golang.org/api/compute/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/gcpclient"
	mockgcpclient "github.com/openshift/hive/pkg/gcpclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
)

func TestGCPCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.GCP = &hivev1gcp.Platform{}
	}).Build()
	actuator := gcpActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestGCPStopAndStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		instances   map[string]int
		setupClient func(*testing.T, *mockgcpclient.MockClient)
	}{
		{
			name:      "stop no running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"TERMINATED": 2, "STOPPING": 2, "STOPPED": 1},
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"TERMINATED": 5, "RUNNING": 2},
			setupClient: func(t *testing.T, c *mockgcpclient.MockClient) {
				c.EXPECT().StopInstance(gomock.Any()).Times(2).Do(
					func(instance *compute.Instance) {
						assert.True(t, instance.Status == "RUNNING")
					},
				)
			},
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"TERMINATED": 5, "STOPPING": 3, "STOPPED": 4, "STAGING": 7, "RUNNING": 3},
			setupClient: func(t *testing.T, c *mockgcpclient.MockClient) {
				c.EXPECT().StopInstance(gomock.Any()).Times(10).Do(
					func(instance *compute.Instance) {
						assert.True(t, instance.Status == "STAGING" || instance.Status == "RUNNING")
					},
				)
			},
		},
		{
			name:      "start no stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"STAGING": 4, "RUNNING": 3},
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"STOPPED": 3, "TERMINATED": 2, "RUNNING": 4},
			setupClient: func(t *testing.T, c *mockgcpclient.MockClient) {
				c.EXPECT().StartInstance(gomock.Any()).Times(5).Do(
					func(instance *compute.Instance) {
						assert.True(t, instance.Status == "STOPPED" || instance.Status == "TERMINATED")
					},
				)
			},
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"STOPPED": 3, "STOPPING": 1, "TERMINATED": 7},
			setupClient: func(t *testing.T, c *mockgcpclient.MockClient) {
				c.EXPECT().StartInstance(gomock.Any()).Times(11).Do(
					func(instance *compute.Instance) {
						assert.True(t, instance.Status == "STOPPED" || instance.Status == "STOPPING" || instance.Status == "TERMINATED")
					},
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			gcpClient := mockgcpclient.NewMockClient(ctrl)
			setupGCPClientInstances(gcpClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, gcpClient)
			}
			actuator := testGCPActuator(gcpClient)
			var err error
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(testClusterDeployment(), nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(testClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			assert.Nil(t, err)
		})
	}

}

func TestGCPMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		expected    bool
		instances   map[string]int
		setupClient func(*testing.T, *mockgcpclient.MockClient)
	}{
		{
			name:      "Stopped - All machines stopped or terminated",
			testFunc:  "MachinesStopped",
			expected:  true,
			instances: map[string]int{"TERMINATED": 3, "STOPPED": 2},
		},
		{
			name:      "Stopped - Some machines pending",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"TERMINATED": 3, "STOPPED": 2, "STAGING": 2},
		},
		{
			name:      "Stopped - machines running",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"RUNNING": 3, "TERMINATED": 2},
		},
		{
			name:      "Running - All machines running",
			testFunc:  "MachinesRunning",
			expected:  true,
			instances: map[string]int{"RUNNING": 3},
		},
		{
			name:      "Running - Some machines pending",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"RUNNING": 3, "STAGING": 1},
		},
		{
			name:      "Running - Some machines stopped or shutting-down",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"RUNNING": 3, "TERMINATED": 2, "STOPPED": 1, "STOPPING": 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			gcpClient := mockgcpclient.NewMockClient(ctrl)
			setupGCPClientInstances(gcpClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, gcpClient)
			}
			actuator := testGCPActuator(gcpClient)
			var err error
			var result bool
			switch test.testFunc {
			case "MachinesStopped":
				result, _, err = actuator.MachinesStopped(testClusterDeployment(), nil, log.New())
			case "MachinesRunning":
				result, _, err = actuator.MachinesRunning(testClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			require.Nil(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func testGCPActuator(gcpClient gcpclient.Client) *gcpActuator {
	return &gcpActuator{
		getGCPClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (gcpclient.Client, error) {
			return gcpClient, nil
		},
	}
}

func setupGCPClientInstances(gcpClient *mockgcpclient.MockClient, statuses map[string]int) {
	instances := []*compute.Instance{}
	for status, count := range statuses {
		for i := 0; i < count; i++ {
			instances = append(instances, &compute.Instance{
				Name:   fmt.Sprintf("%s-%d", status, i),
				Status: status,
			})
		}
	}
	gcpClient.EXPECT().ListComputeInstances(gomock.Any(), gomock.Any()).Times(1).Do(
		func(opts gcpclient.ListComputeInstancesOptions, f func(*compute.InstanceAggregatedList) error) {
			aggregatedList := &compute.InstanceAggregatedList{
				Items: map[string]compute.InstancesScopedList{
					"result": {
						Instances: instances,
					},
				},
			}
			f(aggregatedList)
		},
	).Return(nil)
}

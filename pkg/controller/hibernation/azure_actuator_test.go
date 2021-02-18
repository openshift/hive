package hibernation

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/azureclient"
	mockazureclient "github.com/openshift/hive/pkg/azureclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
)

const (
	azureTestSubscription  = "1111-111-1111-111111"
	azureTestInfraID       = "test-infra-id"
	azureTestResourceGroup = "test-infra-id-rg"
)

func TestAzureCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.Azure = &hivev1azure.Platform{}
	}).Build()
	actuator := azureActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestAzureStopAndStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		instances   map[string]int
		setupClient func(*testing.T, *mockazureclient.MockClient)
	}{
		{
			name:      "stop no running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deallocated": 2, "deallocating": 2, "stopped": 1},
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deallocated": 2, "running": 2},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, map[string]int{"running": 2})
			},
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deallocating": 3, "deallocated": 4, "starting": 1, "running": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, map[string]int{"starting": 1, "running": 3})
			},
		},
		{
			name:      "start no stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"starting": 4, "running": 3},
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"deallocated": 3, "running": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, map[string]int{"deallocated": 3})
			},
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"deallocated": 3, "deallocating": 1, "starting": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, map[string]int{"deallocated": 3, "deallocating": 1})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			azureClient := mockazureclient.NewMockClient(ctrl)
			setupAzureClientInstances(azureClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(testAzureClusterDeployment(), nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(testAzureClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			assert.NoError(t, err)
		})
	}
}

func TestAzureMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		expected    bool
		instances   map[string]int
		setupClient func(*testing.T, *mockazureclient.MockClient)
	}{
		{
			name:      "Stopped - All machines stopped",
			testFunc:  "MachinesStopped",
			expected:  true,
			instances: map[string]int{"deallocated": 3, "stopped": 2},
		},
		{
			name:      "Stopped - Some machines starting",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"deallocated": 3, "stopped": 2, "starting": 2},
		},
		{
			name:      "Stopped - machines running",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"running": 3, "deallocated": 2},
		},
		{
			name:      "Running - All machines running",
			testFunc:  "MachinesRunning",
			expected:  true,
			instances: map[string]int{"running": 3},
		},
		{
			name:      "Running - Some machines starting",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"running": 3, "starting": 1},
		},
		{
			name:      "Running - Some machines deallocated or deallocating",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"running": 3, "deallocated": 2, "stopped": 1, "deallocating": 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			azureClient := mockazureclient.NewMockClient(ctrl)
			setupAzureClientInstances(azureClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			var result bool
			switch test.testFunc {
			case "MachinesStopped":
				result, err = actuator.MachinesStopped(testAzureClusterDeployment(), nil, log.New())
			case "MachinesRunning":
				result, err = actuator.MachinesRunning(testAzureClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func testAzureClusterDeployment() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				InfraID: azureTestInfraID,
			},
		},
	}
}

func testAzureActuator(azureClient azureclient.Client) *azureActuator {
	return &azureActuator{
		azureClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (azureclient.Client, error) {
			return azureClient, nil
		},
	}
}

func setupAzureClientInstances(client *mockazureclient.MockClient, instances map[string]int) {
	vms := []compute.VirtualMachine{}
	for state, count := range instances {
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("%s-%d", state, i)
			instanceViewStatus := []compute.InstanceViewStatus{
				{
					Code: pointer.StringPtr("PowerState/" + state),
				},
			}
			vms = append(vms, compute.VirtualMachine{
				Name: pointer.StringPtr(name),
				ID:   pointer.StringPtr(fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", azureTestSubscription, azureTestResourceGroup, name)),
				VirtualMachineProperties: &compute.VirtualMachineProperties{
					InstanceView: &compute.VirtualMachineInstanceView{
						Statuses: &instanceViewStatus,
					},
				},
			})
		}
	}
	result := compute.NewVirtualMachineListResultPage(func(ctx context.Context, result compute.VirtualMachineListResult) (compute.VirtualMachineListResult, error) {
		if result.Value == nil {
			return compute.VirtualMachineListResult{Value: &vms}, nil
		}
		return compute.VirtualMachineListResult{}, nil
	})
	result.Next()
	client.EXPECT().ListAllVirtualMachines(gomock.Any(), "true").Times(1).Return(result, nil)
}

func setupAzureDeallocateCalls(client *mockazureclient.MockClient, instances map[string]int) {
	for state, count := range instances {
		for i := 0; i < count; i++ {
			client.EXPECT().DeallocateVirtualMachine(gomock.Any(), azureTestResourceGroup, fmt.Sprintf("%s-%d", state, i)).Times(1)
		}
	}
}

func setupAzureStartCalls(client *mockazureclient.MockClient, instances map[string]int) {
	for state, count := range instances {
		for i := 0; i < count; i++ {
			client.EXPECT().StartVirtualMachine(gomock.Any(), azureTestResourceGroup, fmt.Sprintf("%s-%d", state, i)).Times(1)
		}
	}
}

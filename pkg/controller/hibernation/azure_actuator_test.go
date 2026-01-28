package hibernation

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute"
	"go.uber.org/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/utils/ptr"
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
		name              string
		testFunc          string
		instances         map[string]int
		setupClient       func(*testing.T, *mockazureclient.MockClient)
		resourceGroupName string
		expectError       bool
	}{
		{
			name:              "stop no running instances",
			testFunc:          "StopMachines",
			instances:         map[string]int{"deallocated": 2, "deallocating": 2, "stopped": 1},
			resourceGroupName: "anything",
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deallocated": 2, "running": 2},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, map[string]int{"running": 2}, azureTestResourceGroup)
			},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"deallocating": 3, "deallocated": 4, "starting": 1, "running": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, map[string]int{"starting": 1, "running": 3}, "custom-rg")
			},
			resourceGroupName: "custom-rg",
		},
		{
			name:              "start no stopped instances",
			testFunc:          "StartMachines",
			instances:         map[string]int{"starting": 4, "running": 3},
			resourceGroupName: "anything",
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"deallocated": 3, "running": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, map[string]int{"deallocated": 3}, azureTestResourceGroup)
			},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"deallocated": 3, "deallocating": 1, "starting": 3},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, map[string]int{"deallocated": 3, "deallocating": 1}, "custom-rg")
			},
			resourceGroupName: "custom-rg",
		},
		{
			name:        "error: missing resource group (stop)",
			testFunc:    "StopMachines",
			instances:   map[string]int{"deallocating": 3, "deallocated": 4, "starting": 1, "running": 3},
			expectError: true,
		},
		{
			name:        "error: missing resource group (start)",
			testFunc:    "StartMachines",
			instances:   map[string]int{"deallocated": 3, "deallocating": 1, "starting": 3},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			azureClient := mockazureclient.NewMockClient(ctrl)
			setupAzureClientInstances(azureClient, test.instances, test.resourceGroupName)
			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			var opts []Option
			if test.resourceGroupName != "" {
				opts = []Option{withClusterMetadataResourceGroupName(test.resourceGroupName)}
			}
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(testAzureClusterDeployment(opts...), nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(testAzureClusterDeployment(opts...), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			if test.expectError {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), "Azure ResourceGroupName is unset!")
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAzureMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name              string
		testFunc          string
		expected          bool
		instances         map[string]int
		setupClient       func(*testing.T, *mockazureclient.MockClient)
		resourceGroupName string
		expectError       bool
	}{
		{
			name:              "Stopped - All machines stopped",
			testFunc:          "MachinesStopped",
			expected:          true,
			instances:         map[string]int{"deallocated": 3, "stopped": 2},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:              "Stopped - Some machines starting",
			testFunc:          "MachinesStopped",
			expected:          false,
			instances:         map[string]int{"deallocated": 3, "stopped": 2, "starting": 2},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:              "Stopped - machines running",
			testFunc:          "MachinesStopped",
			expected:          false,
			instances:         map[string]int{"running": 3, "deallocated": 2},
			resourceGroupName: "custom-rg",
		},
		{
			name:              "Running - All machines running",
			testFunc:          "MachinesRunning",
			expected:          true,
			instances:         map[string]int{"running": 3},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:              "Running - Some machines starting",
			testFunc:          "MachinesRunning",
			expected:          false,
			instances:         map[string]int{"running": 3, "starting": 1},
			resourceGroupName: azureTestResourceGroup,
		},
		{
			name:              "Running - Some machines deallocated or deallocating",
			testFunc:          "MachinesRunning",
			expected:          false,
			instances:         map[string]int{"running": 3, "deallocated": 2, "stopped": 1, "deallocating": 3},
			resourceGroupName: "custom-rg",
		},
		{
			name:        "error: no resource group (stopped)",
			testFunc:    "MachinesStopped",
			expected:    false,
			instances:   map[string]int{"deallocated": 3, "stopped": 2, "starting": 2},
			expectError: true,
		},
		{
			name:        "error: no resource group (running)",
			testFunc:    "MachinesRunning",
			expected:    false,
			instances:   map[string]int{"running": 3, "deallocated": 2, "stopped": 1, "deallocating": 3},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			azureClient := mockazureclient.NewMockClient(ctrl)
			setupAzureClientInstances(azureClient, test.instances, test.resourceGroupName)
			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			var opts []Option
			if test.resourceGroupName != "" {
				opts = []Option{withClusterMetadataResourceGroupName(test.resourceGroupName)}
			}
			var result bool
			switch test.testFunc {
			case "MachinesStopped":
				result, _, err = actuator.MachinesStopped(testAzureClusterDeployment(opts...), nil, log.New())
			case "MachinesRunning":
				result, _, err = actuator.MachinesRunning(testAzureClusterDeployment(opts...), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			if test.expectError {
				if assert.Error(t, err) {
					assert.Contains(t, err.Error(), "Azure ResourceGroupName is unset!")
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expected, result)
		})
	}
}

type Option func(*hivev1.ClusterDeployment)

func testAzureClusterDeployment(opts ...Option) *hivev1.ClusterDeployment {
	cd := &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterMetadata: &hivev1.ClusterMetadata{
				InfraID: azureTestInfraID,
			},
		},
	}
	for _, opt := range opts {
		opt(cd)
	}
	return cd
}

func withClusterMetadataResourceGroupName(rg string) Option {
	return func(cd *hivev1.ClusterDeployment) {
		if cd.Spec.ClusterMetadata == nil {
			cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
		}
		if cd.Spec.ClusterMetadata.Platform == nil {
			cd.Spec.ClusterMetadata.Platform = &hivev1.ClusterPlatformMetadata{}
		}
		if cd.Spec.ClusterMetadata.Platform.Azure == nil {
			cd.Spec.ClusterMetadata.Platform.Azure = &hivev1azure.Metadata{}
		}
		cd.Spec.ClusterMetadata.Platform.Azure.ResourceGroupName = ptr.To(rg)
	}
}

func testAzureActuator(azureClient azureclient.Client) *azureActuator {
	return &azureActuator{
		azureClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (azureclient.Client, error) {
			return azureClient, nil
		},
	}
}

func setupAzureClientInstances(client *mockazureclient.MockClient, instances map[string]int, rgname string) {
	vms := []compute.VirtualMachine{}
	for state, count := range instances {
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("%s-%d", state, i)
			instanceViewStatus := []compute.InstanceViewStatus{
				{
					Code: ptr.To("PowerState/" + state),
				},
			}
			vms = append(vms, compute.VirtualMachine{
				Name: ptr.To(name),
				ID:   ptr.To(fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", azureTestSubscription, rgname, name)),
				VirtualMachineProperties: &compute.VirtualMachineProperties{
					InstanceView: &compute.VirtualMachineInstanceView{
						Statuses: &instanceViewStatus,
					},
				},
			})
		}
	}
	cur := compute.VirtualMachineListResult{}
	result := compute.NewVirtualMachineListResultPage(cur, func(ctx context.Context, result compute.VirtualMachineListResult) (compute.VirtualMachineListResult, error) {
		if result.Value == nil {
			return compute.VirtualMachineListResult{Value: &vms}, nil
		}
		return compute.VirtualMachineListResult{}, nil
	})
	result.Next()
	client.EXPECT().ListAllVirtualMachines(gomock.Any(), "true").Times(1).Return(result, nil)
}

func setupAzureDeallocateCalls(client *mockazureclient.MockClient, instances map[string]int, rgname string) {
	for state, count := range instances {
		for i := 0; i < count; i++ {
			client.EXPECT().DeallocateVirtualMachine(gomock.Any(), rgname, fmt.Sprintf("%s-%d", state, i)).Times(1)
		}
	}
}

func setupAzureStartCalls(client *mockazureclient.MockClient, instances map[string]int, rgname string) {
	for state, count := range instances {
		for i := 0; i < count; i++ {
			client.EXPECT().StartVirtualMachine(gomock.Any(), rgname, fmt.Sprintf("%s-%d", state, i)).Times(1)
		}
	}
}

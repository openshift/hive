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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"
	"github.com/openshift/hive/pkg/azureclient"
	mockazureclient "github.com/openshift/hive/pkg/azureclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
)

const (
	azureTestSubscription  = "1111-111-1111-111111"
	azureTestInfraID       = "test-infra-id"
	azureTestResourceGroup = "test-infra-id-rg"
)

type azureInstanceGroup struct {
	state             string
	count             int
	resourceGroupName string
}

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
		instances         []azureInstanceGroup
		setupClient       func(*testing.T, *mockazureclient.MockClient)
		clusterDeployment *hivev1.ClusterDeployment
	}{
		{
			name:     "stop no running instances",
			testFunc: "StopMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 2),
				newDefaultAzureInstanceGroup("deallocating", 2),
				newDefaultAzureInstanceGroup("stopped", 2),
			},
		},
		{
			name:     "stop running instances",
			testFunc: "StopMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 2),
				newDefaultAzureInstanceGroup("running", 2),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, []azureInstanceGroup{
					newDefaultAzureInstanceGroup("running", 2),
				})
			},
		},
		{
			name:     "stop pending and running instances",
			testFunc: "StopMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocating", 3),
				newDefaultAzureInstanceGroup("deallocated", 4),
				newDefaultAzureInstanceGroup("starting", 1),
				newDefaultAzureInstanceGroup("running", 3),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, []azureInstanceGroup{
					newDefaultAzureInstanceGroup("starting", 1),
					newDefaultAzureInstanceGroup("running", 3),
				})
			},
		},
		{
			name:     "start no stopped instances",
			testFunc: "StartMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("starting", 4),
				newDefaultAzureInstanceGroup("running", 3),
			},
		},
		{
			name:     "start stopped instances",
			testFunc: "StartMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 3),
				newDefaultAzureInstanceGroup("running", 3),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, []azureInstanceGroup{
					newDefaultAzureInstanceGroup("deallocated", 3),
				})
			},
		},
		{
			name:     "start stopped and stopping instances",
			testFunc: "StartMachines",
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 3),
				newDefaultAzureInstanceGroup("deallocating", 1),
				newDefaultAzureInstanceGroup("starting", 3),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, []azureInstanceGroup{
					newDefaultAzureInstanceGroup("deallocated", 3),
					newDefaultAzureInstanceGroup("deallocating", 1),
				})
			},
		},
		{
			name:     "use resource group name on start",
			testFunc: "StartMachines",
			instances: []azureInstanceGroup{
				newAzureInstanceGroup("stopped", 3, "manual-resourcegroup"),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureStartCalls(c, []azureInstanceGroup{
					newAzureInstanceGroup("stopped", 3, "manual-resourcegroup"),
				})
			},
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testAzureClusterDeployment()
				cd.Spec.Platform.Azure = &hivev1azure.Platform{
					ResourceGroupName: "manual-resourcegroup",
				}
				return cd
			}(),
		},
		{
			name:     "use resource group name on stop",
			testFunc: "StopMachines",
			instances: []azureInstanceGroup{
				newAzureInstanceGroup("running", 3, "manual-resourcegroup"),
			},
			setupClient: func(t *testing.T, c *mockazureclient.MockClient) {
				setupAzureDeallocateCalls(c, []azureInstanceGroup{
					newAzureInstanceGroup("running", 3, "manual-resourcegroup"),
				})
			},
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testAzureClusterDeployment()
				cd.Spec.Platform.Azure = &hivev1azure.Platform{
					ResourceGroupName: "manual-resourcegroup",
				}
				return cd
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			azureClient := mockazureclient.NewMockClient(ctrl)

			cd := testAzureClusterDeployment()
			if test.clusterDeployment != nil {
				cd = test.clusterDeployment
			}

			setupAzureClientInstances(azureClient, test.instances)

			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(cd, nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(cd, nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			assert.NoError(t, err)
		})
	}
}

func TestAzureMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name              string
		testFunc          string
		expected          bool
		instances         []azureInstanceGroup
		instanceSetup     func(*mockazureclient.MockClient, []azureInstanceGroup)
		setupClient       func(*testing.T, *mockazureclient.MockClient)
		clusterDeployment *hivev1.ClusterDeployment
	}{
		{
			name:     "Stopped - All machines stopped",
			testFunc: "MachinesStopped",
			expected: true,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 3),
				newDefaultAzureInstanceGroup("stopped", 2),
			},
		},
		{
			name:     "Stopped - Some machines starting",
			testFunc: "MachinesStopped",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("deallocated", 3),
				newDefaultAzureInstanceGroup("stopped", 2),
				newDefaultAzureInstanceGroup("starting", 2),
			},
		},
		{
			name:     "Stopped - machines running",
			testFunc: "MachinesStopped",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newDefaultAzureInstanceGroup("deallocated", 2),
			},
		},
		{
			name:     "Running - All machines running",
			testFunc: "MachinesRunning",
			expected: true,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
			},
		},
		{
			name:     "Running - Some machines starting",
			testFunc: "MachinesRunning",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newDefaultAzureInstanceGroup("starting", 1),
			},
		},
		{
			name:     "Running - Some machines deallocated or deallocating",
			testFunc: "MachinesRunning",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newDefaultAzureInstanceGroup("deallocated", 2),
				newDefaultAzureInstanceGroup("stopped", 1),
				newDefaultAzureInstanceGroup("deallocating", 3),
			},
		},
		{
			name:     "Stopped - mixed with other resource groups",
			testFunc: "MachinesRunning",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newAzureInstanceGroup("stopped", 4, "manual-resourcegroup"),
			},
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform.Azure = &hivev1azure.Platform{
					ResourceGroupName: "manual-resourcegroup",
				}
				return cd
			}(),
		},
		{
			name:     "Half Running - mixed with other resource groups",
			testFunc: "MachinesRunning",
			expected: false,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newAzureInstanceGroup("stopped", 4, "manual-resourcegroup"),
				newAzureInstanceGroup("running", 3, "manual-resourcegroup"),
			},
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform.Azure = &hivev1azure.Platform{
					ResourceGroupName: "manual-resourcegroup",
				}
				return cd
			}(),
		},
		{
			name:     "Fully Running - mixed with other resource groups",
			testFunc: "MachinesRunning",
			expected: true,
			instances: []azureInstanceGroup{
				newDefaultAzureInstanceGroup("running", 3),
				newAzureInstanceGroup("running", 4, "manual-resourcegroup"),
				newAzureInstanceGroup("stopped", 3, "other-resourcegroup"),
			},
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.Platform.Azure = &hivev1azure.Platform{
					ResourceGroupName: "manual-resourcegroup",
				}
				return cd
			}(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cd := testAzureClusterDeployment()
			if test.clusterDeployment != nil {
				cd = test.clusterDeployment
			}

			azureClient := mockazureclient.NewMockClient(ctrl)
			if test.instanceSetup != nil {
				test.instanceSetup(azureClient, test.instances)
			} else {
				setupAzureClientInstances(azureClient, test.instances)
			}

			if test.setupClient != nil {
				test.setupClient(t, azureClient)
			}
			actuator := testAzureActuator(azureClient)
			var err error
			var result bool
			switch test.testFunc {
			case "MachinesStopped":
				result, err = actuator.MachinesStopped(cd, nil, log.New())
			case "MachinesRunning":
				result, err = actuator.MachinesRunning(cd, nil, log.New())
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

func setupAzureClientInstances(client *mockazureclient.MockClient, instances []azureInstanceGroup) {
	vms := []compute.VirtualMachine{}

	for _, azInstance := range instances {
		for i := 0; i < azInstance.count; i++ {
			name := fmt.Sprintf("%s-%d", azInstance.state, i)
			instanceViewStatus := []compute.InstanceViewStatus{
				{
					Code: pointer.StringPtr("PowerState/" + azInstance.state),
				},
			}

			vms = append(vms, compute.VirtualMachine{
				Name: pointer.StringPtr(name),
				ID:   pointer.StringPtr(fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s", azureTestSubscription, azInstance.resourceGroupName, name)),
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

func setupAzureDeallocateCalls(client *mockazureclient.MockClient, instances []azureInstanceGroup) {
	for _, azInstances := range instances {
		for i := 0; i < azInstances.count; i++ {
			client.EXPECT().DeallocateVirtualMachine(gomock.Any(), azInstances.resourceGroupName, fmt.Sprintf("%s-%d", azInstances.state, i)).Times(1)
		}
	}
}

func setupAzureStartCalls(client *mockazureclient.MockClient, instances []azureInstanceGroup) {
	for _, azInstances := range instances {
		for i := 0; i < azInstances.count; i++ {
			client.EXPECT().StartVirtualMachine(gomock.Any(), azInstances.resourceGroupName, fmt.Sprintf("%s-%d", azInstances.state, i)).Times(1)
		}
	}
}

func newDefaultAzureInstanceGroup(state string, count int) azureInstanceGroup {
	return newAzureInstanceGroup(state, count, azureTestResourceGroup)
}

func newAzureInstanceGroup(state string, count int, resourceGroupName string) azureInstanceGroup {
	return azureInstanceGroup{state: state, count: count, resourceGroupName: resourceGroupName}
}

package hibernation

import (
	"errors"
	"fmt"
	"sort"
	"testing"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/responses"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1alibabacloud "github.com/openshift/hive/apis/hive/v1/alibabacloud"
	"github.com/openshift/hive/pkg/alibabaclient"
	mockalibabacloud "github.com/openshift/hive/pkg/alibabaclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestAlibabaCloudCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.AlibabaCloud = &hivev1alibabacloud.Platform{}
	}).Build()
	actuator := alibabaCloudActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestAlibabaCloudStopAndStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		instances   map[string]int
		setupClient func(*testing.T, *mockalibabacloud.MockAPI)
		expectErr   bool
	}{
		{
			name:      "stop no running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"Stopping": 2, "Stopped": 1},
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"Stopping": 5, "Running": 2},
			setupClient: func(t *testing.T, c *mockalibabacloud.MockAPI) {
				c.EXPECT().StopInstances(gomock.Any()).Times(1).Do(
					func(request *ecs.StopInstancesRequest) {
						matchAlibabaInstanceIDs(t, request.InstanceId, map[string]int{"Running": 2})
					}).Return(nil, nil)
			},
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"Stopping": 3, "Stopped": 4, "Pending": 7, "Running": 3},
			setupClient: func(t *testing.T, c *mockalibabacloud.MockAPI) {
				c.EXPECT().StopInstances(gomock.Any()).Do(
					func(request *ecs.StopInstancesRequest) {
						matchAlibabaInstanceIDs(t, request.InstanceId, map[string]int{"Pending": 7, "Running": 3})
					}).Return(nil, nil)
			},
		},
		{
			name:      "start no stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"Pending": 4, "Running": 3},
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"Stopped": 3, "Running": 4},
			setupClient: func(t *testing.T, c *mockalibabacloud.MockAPI) {
				c.EXPECT().StartInstances(gomock.Any()).Times(1).Do(
					func(request *ecs.StartInstancesRequest) {
						matchAlibabaInstanceIDs(t, request.InstanceId, map[string]int{"Stopped": 3})
					}).Return(nil, nil)
			},
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"Stopped": 3, "Stopping": 1, "Running": 7},
			setupClient: func(t *testing.T, c *mockalibabacloud.MockAPI) {
				c.EXPECT().StartInstances(gomock.Any()).Times(1).Do(
					func(request *ecs.StartInstancesRequest) {
						matchAlibabaInstanceIDs(t, request.InstanceId, map[string]int{"Stopped": 3, "Stopping": 1})
					}).Return(nil, nil)
			},
		},
		{
			name:      "unable to DescribeInstances",
			testFunc:  "StopMachines",
			instances: map[string]int{"Stopping": 5, "Running": 2},
			setupClient: func(t *testing.T, c *mockalibabacloud.MockAPI) {
				c.EXPECT().DescribeInstances(gomock.Any()).Times(1).Return(nil, errors.New("cannot list instances"))
			},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			alibabaCloudClient := mockalibabacloud.NewMockAPI(ctrl)
			if !test.expectErr {
				setupAlibabaCloudClientInstances(alibabaCloudClient, test.instances)
			}
			if test.setupClient != nil {
				test.setupClient(t, alibabaCloudClient)
			}
			actuator := testAlibabaCloudActuator(alibabaCloudClient)
			var err error
			switch test.testFunc {
			case "StopMachines":
				err = actuator.StopMachines(testAlibabaCloudClusterDeployment(), nil, log.New())
			case "StartMachines":
				err = actuator.StartMachines(testAlibabaCloudClusterDeployment(), nil, log.New())
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

func TestAlibabaCloudMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name              string
		testFunc          string
		expectedRemaining []string
		expectedResult    bool
		instances         map[string]int
		setupClient       func(*testing.T, *mockalibabacloud.MockAPI)
	}{
		{
			name:           "Stopped - All machines stopped",
			testFunc:       "MachinesStopped",
			expectedResult: true,
			instances:      map[string]int{"Stopped": 2},
		},
		{
			name:              "Stopped - Some machines pending",
			testFunc:          "MachinesStopped",
			expectedResult:    false,
			expectedRemaining: []string{"Pending-0", "Pending-1"},
			instances:         map[string]int{"Stopped": 2, "Pending": 2},
		},
		{
			name:              "Stopped - machines running",
			testFunc:          "MachinesStopped",
			expectedResult:    false,
			expectedRemaining: []string{"Running-0", "Running-1", "Running-2"},
			instances:         map[string]int{"Running": 3, "Stopped": 2},
		},
		{
			name:              "Running - All machines running",
			testFunc:          "MachinesRunning",
			expectedResult:    true,
			expectedRemaining: []string{},
			instances:         map[string]int{"Running": 3},
		},
		{
			name:              "Running - Some machines pending",
			testFunc:          "MachinesRunning",
			expectedResult:    false,
			expectedRemaining: []string{"Pending-0"},
			instances:         map[string]int{"Running": 3, "Pending": 1},
		},
		{
			name:              "Running - Some machines stopped or shutting-down",
			testFunc:          "MachinesRunning",
			expectedResult:    false,
			expectedRemaining: []string{"Stopping-0", "Stopping-1", "Stopping-2", "Stopped-0"},
			instances:         map[string]int{"Running": 3, "Stopped": 1, "Stopping": 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			alibabaCloudClient := mockalibabacloud.NewMockAPI(ctrl)
			setupAlibabaCloudClientInstances(alibabaCloudClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, alibabaCloudClient)
			}
			actuator := testAlibabaCloudActuator(alibabaCloudClient)
			var err error
			var result bool
			var remaining []string
			switch test.testFunc {
			case "MachinesStopped":
				result, remaining, err = actuator.MachinesStopped(testAlibabaCloudClusterDeployment(), nil, log.New())
			case "MachinesRunning":
				result, remaining, err = actuator.MachinesRunning(testAlibabaCloudClusterDeployment(), nil, log.New())
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

func testAlibabaCloudActuator(alibabaCloudClient alibabaclient.API) *alibabaCloudActuator {
	return &alibabaCloudActuator{
		alibabaCloudClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (alibabaclient.API, error) {
			return alibabaCloudClient, nil
		},
	}
}

func setupAlibabaCloudClientInstances(alibabaCloudClient *mockalibabacloud.MockAPI, statuses map[string]int) {
	instances := []ecs.Instance{}
	for status, count := range statuses {
		for i := 0; i < count; i++ {
			instances = append(instances, ecs.Instance{
				InstanceName: fmt.Sprintf("%s-%d", status, i),
				InstanceId:   fmt.Sprintf("%s-%d", status, i),
				Status:       status,
			})
		}
	}

	response := &ecs.DescribeInstancesResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	response.Instances.Instance = instances

	alibabaCloudClient.EXPECT().DescribeInstances(gomock.Any()).Times(1).Return(response, nil)
}

func testAlibabaCloudClusterDeployment() *hivev1.ClusterDeployment {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder("testns", "testalibabacluster", scheme)
	return cdBuilder.Build(
		testcd.WithAlibabaCloudPlatform(&hivev1alibabacloud.Platform{Region: "cn-hangzhou"}),
		testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: "testalibabacluster-foobarbaz"}),
	)
}

func matchAlibabaInstanceIDs(t *testing.T, actual *[]string, states map[string]int) {
	expected := sets.NewString()
	for state, count := range states {
		for i := 0; i < count; i++ {
			expected.Insert(fmt.Sprintf("%s-%d", state, i))
		}
	}
	actualSet := sets.NewString()
	for _, a := range *actual {
		actualSet.Insert(a)
	}
	assert.True(t, expected.Equal(actualSet), "Unexpected set of instance IDs: %v", actualSet.List())
}

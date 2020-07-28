package hibernation

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	mockawsclient "github.com/openshift/hive/pkg/awsclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
)

func TestCanHandle(t *testing.T) {
	cd := testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.Platform.AWS = &hivev1aws.Platform{}
	}).Build()
	actuator := awsActuator{}
	assert.True(t, actuator.CanHandle(cd))

	cd = testcd.BasicBuilder().Build()
	assert.False(t, actuator.CanHandle(cd))
}

func TestStopAndStartMachines(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		instances   map[string]int
		setupClient func(*testing.T, *mockawsclient.MockClient)
	}{
		{
			name:      "stop no running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"terminated": 2, "stopping": 2, "stopped": 1},
		},
		{
			name:      "stop running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"terminated": 2, "running": 2},
			setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
				c.EXPECT().StopInstances(gomock.Any()).Do(
					func(input *ec2.StopInstancesInput) {
						matchInstanceIDs(t, input.InstanceIds, map[string]int{"running": 2})
					}).Return(nil, nil)
			},
		},
		{
			name:      "stop pending and running instances",
			testFunc:  "StopMachines",
			instances: map[string]int{"terminated": 5, "shutting-down": 3, "stopped": 4, "pending": 1, "running": 3},
			setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
				c.EXPECT().StopInstances(gomock.Any()).Do(
					func(input *ec2.StopInstancesInput) {
						matchInstanceIDs(t, input.InstanceIds, map[string]int{"pending": 1, "running": 3})
					}).Return(nil, nil)
			},
		},
		{
			name:      "start no stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"terminated": 3, "pending": 4, "running": 3},
		},
		{
			name:      "start stopped instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"stopped": 3, "terminated": 2, "running": 3},
			setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
				c.EXPECT().StartInstances(gomock.Any()).Do(
					func(input *ec2.StartInstancesInput) {
						matchInstanceIDs(t, input.InstanceIds, map[string]int{"stopped": 3})
					}).Return(nil, nil)
			},
		},
		{
			name:      "start stopped and stopping instances",
			testFunc:  "StartMachines",
			instances: map[string]int{"stopped": 3, "stopping": 1, "terminated": 3},
			setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
				c.EXPECT().StartInstances(gomock.Any()).Do(
					func(input *ec2.StartInstancesInput) {
						matchInstanceIDs(t, input.InstanceIds, map[string]int{"stopped": 3, "stopping": 1})
					}).Return(nil, nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			awsClient := mockawsclient.NewMockClient(ctrl)
			setupClientInstances(awsClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, awsClient)
			}
			actuator := testAWSActuator(awsClient)
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

func TestMachinesStoppedAndRunning(t *testing.T) {
	tests := []struct {
		name        string
		testFunc    string
		expected    bool
		instances   map[string]int
		setupClient func(*testing.T, *mockawsclient.MockClient)
	}{
		{
			name:      "Stopped - All machines stopped or terminated",
			testFunc:  "MachinesStopped",
			expected:  true,
			instances: map[string]int{"terminated": 3, "stopped": 2},
		},
		{
			name:      "Stopped - Some machines pending",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"terminated": 3, "stopped": 2, "pending": 2},
		},
		{
			name:      "Stopped - machines running",
			testFunc:  "MachinesStopped",
			expected:  false,
			instances: map[string]int{"running": 3, "terminated": 2},
		},
		{
			name:      "Running - All machines running or terminated",
			testFunc:  "MachinesRunning",
			expected:  true,
			instances: map[string]int{"running": 3, "terminated": 2},
		},
		{
			name:      "Running - Some machines pending",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"running": 3, "terminated": 2, "pending": 1},
		},
		{
			name:      "Running - Some machines stopped or shutting-down",
			testFunc:  "MachinesRunning",
			expected:  false,
			instances: map[string]int{"running": 3, "terminated": 2, "stopped": 1, "shutting-down": 3},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			awsClient := mockawsclient.NewMockClient(ctrl)
			setupClientInstances(awsClient, test.instances)
			if test.setupClient != nil {
				test.setupClient(t, awsClient)
			}
			actuator := testAWSActuator(awsClient)
			var err error
			var result bool
			switch test.testFunc {
			case "MachinesStopped":
				result, err = actuator.MachinesStopped(testClusterDeployment(), nil, log.New())
			case "MachinesRunning":
				result, err = actuator.MachinesRunning(testClusterDeployment(), nil, log.New())
			default:
				t.Fatal("Invalid function to test")
			}
			require.Nil(t, err)
			assert.Equal(t, test.expected, result)
		})
	}
}

func matchInstanceIDs(t *testing.T, actual []*string, states map[string]int) {
	expected := sets.NewString()
	for state, count := range states {
		for i := 0; i < count; i++ {
			expected.Insert(fmt.Sprintf("%s-%d", state, i))
		}
	}
	actualSet := sets.NewString()
	for _, a := range actual {
		actualSet.Insert(aws.StringValue(a))
	}
	assert.True(t, expected.Equal(actualSet), "Unexpected set of instance IDs: %v", actualSet.List())
}

func testAWSActuator(awsClient awsclient.Client) *awsActuator {
	return &awsActuator{
		awsClientFn: func(*hivev1.ClusterDeployment, client.Client, log.FieldLogger) (awsclient.Client, error) {
			return awsClient, nil
		},
	}
}

func testClusterDeployment() *hivev1.ClusterDeployment {
	return testcd.BasicBuilder().Options(func(cd *hivev1.ClusterDeployment) {
		cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
			InfraID: "abcd1234",
		}
	}).Build()
}

func setupClientInstances(awsClient *mockawsclient.MockClient, states map[string]int) {
	instances := []*ec2.Instance{}
	for state, count := range states {
		for i := 0; i < count; i++ {
			instances = append(instances, &ec2.Instance{
				InstanceId: aws.String(fmt.Sprintf("%s-%d", state, i)),
				State: &ec2.InstanceState{
					Name: aws.String(state),
				},
			})
		}
	}
	reservation := &ec2.Reservation{Instances: instances}
	reservations := []*ec2.Reservation{reservation}
	awsClient.EXPECT().DescribeInstances(gomock.Any()).Times(1).Return(
		&ec2.DescribeInstancesOutput{
			Reservations: reservations,
		},
		nil)
}

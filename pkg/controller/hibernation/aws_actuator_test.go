package hibernation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	machineapi "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	mockawsclient "github.com/openshift/hive/pkg/awsclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
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

func TestStopPreemptibleMachines(t *testing.T) {
	tests := []struct {
		name string
		// number of on-demand and spot instances
		onDemand int
		spot     int

		setupClient func(*testing.T, *mockawsclient.MockClient)
	}{{
		name:     "only on-demand instances",
		onDemand: 1,
		setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
			c.EXPECT().StopInstances(gomock.Any()).Do(
				func(input *ec2.StopInstancesInput) {
					input.InstanceIds = aws.StringSlice([]string{"onDemand-0"})
				}).Return(nil, nil)
		},
	}, {
		name: "only spot instances",
		spot: 1,
		setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
			c.EXPECT().TerminateInstances(gomock.Any()).Do(
				func(input *ec2.TerminateInstancesInput) {
					input.InstanceIds = aws.StringSlice([]string{"spot-0"})
				}).Return(nil, nil)
		},
	}, {
		name:     "mix of spot and on-demand instances",
		onDemand: 1,
		spot:     2,
		setupClient: func(t *testing.T, c *mockawsclient.MockClient) {
			c.EXPECT().StopInstances(gomock.Any()).Do(
				func(input *ec2.StopInstancesInput) {
					input.InstanceIds = aws.StringSlice([]string{"onDemand-0"})
				}).Return(nil, nil)
			c.EXPECT().TerminateInstances(gomock.Any()).Do(
				func(input *ec2.TerminateInstancesInput) {
					input.InstanceIds = aws.StringSlice([]string{"spot-0", "spot-1"})
				}).Return(nil, nil)
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			awsClient := mockawsclient.NewMockClient(ctrl)
			setupOnDemandAndSpotInstances(awsClient, test.onDemand, test.spot)
			if test.setupClient != nil {
				test.setupClient(t, awsClient)
			}
			actuator := testAWSActuator(awsClient)
			err := actuator.StopMachines(testClusterDeployment(), nil, log.New())
			assert.NoError(t, err)
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

func TestReplacePreemptibleMachines(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	testcd := testcd.FullBuilder(namespace, cdName, scheme).Options(
		testcd.Installed(),
		testcd.WithClusterVersion("4.4.9"),
		testcd.WithCondition(hivev1.ClusterDeploymentCondition{
			Type:               hivev1.ClusterHibernatingCondition,
			Reason:             hivev1.HibernatingReasonHibernating,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
		}),
	).Build()

	tests := []struct {
		name             string
		existingMachines []runtime.Object

		expectedReplaced bool
		expectedErr      string
		expectedMachines []string // machines that exist after replacement
	}{{
		name: "no machines",

		expectedMachines: []string{},
	}, {
		name: "all on-demand machines",
		existingMachines: []runtime.Object{
			testMachine("ondemand-1", false, "Running", time.Now()),
			testMachine("ondemand-2", false, "Provisioning", time.Now()),
			testMachine("ondemand-3", false, "Provisioned", time.Now()),
		},
		expectedMachines: []string{"ondemand-1", "ondemand-2", "ondemand-3"},
	}, {
		name: "all running spot instances, updated after hibernation",
		existingMachines: []runtime.Object{
			testMachine("spot-1", true, "Running", time.Now()),
			testMachine("spot-2", true, "Provisioning", time.Now()),
			testMachine("spot-3", true, "Provisioned", time.Now()),
		},
		expectedMachines: []string{"spot-1", "spot-2", "spot-3"},
	}, {
		name: "all running spot instances, updated before hibernation",
		existingMachines: []runtime.Object{
			testMachine("spot-1", true, "Running", time.Now().Add(-1*time.Hour)),
			testMachine("spot-2", true, "Provisioning", time.Now().Add(-1*time.Hour)),
			testMachine("spot-3", true, "Provisioned", time.Now().Add(-1*time.Hour)),
		},
		expectedReplaced: true,
		expectedMachines: []string{},
	}, {
		name: "some failed spot instances",
		existingMachines: []runtime.Object{
			testMachine("spot-1", true, "Running", time.Now().Add(-1*time.Hour)),
			testMachine("spot-2", true, "Failed", time.Now()),
			testMachine("spot-3", true, "Provisioned", time.Now()),
		},
		expectedReplaced: true,
		expectedMachines: []string{"spot-3"},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actuator := testAWSActuator(nil)
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existingMachines...).Build()

			replaced, err := actuator.ReplaceMachines(testcd, c, logger)
			if test.expectedErr != "" {
				assert.EqualError(t, err, test.expectedErr)
			} else {
				assert.Equal(t, test.expectedReplaced, replaced)
				machineList := &machineapi.MachineList{}
				err = c.List(context.TODO(), machineList,
					client.InNamespace(machineAPINamespace),
				)
				require.NoError(t, err)

				machines := sets.NewString()
				for _, m := range machineList.Items {
					machines.Insert(m.GetName())
				}
				assert.Equal(t, test.expectedMachines, machines.List())
			}
		})
	}
}

func testMachine(name string, interruptible bool, phase string, lastUpdated time.Time) *machineapi.Machine {
	labels := map[string]string{}
	if interruptible {
		labels[machineAPIInterruptibleLabel] = ""
	}

	ms := machineapi.Machine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: machineapi.SchemeGroupVersion.String(),
			Kind:       "Machine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: machineAPINamespace,
			Labels:    labels,
		},
		Status: machineapi.MachineStatus{
			Phase:       &phase,
			LastUpdated: &metav1.Time{Time: lastUpdated},
		},
	}
	return &ms
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

func setupOnDemandAndSpotInstances(awsClient *mockawsclient.MockClient, onDemand, spot int) {
	instances := []*ec2.Instance{}
	for i := 0; i < onDemand; i++ {
		instances = append(instances, &ec2.Instance{
			InstanceId: aws.String(fmt.Sprintf("%s-%d", "onDemand", i)),
			State: &ec2.InstanceState{
				Name: aws.String("running"),
			},
		})
	}
	for i := 0; i < spot; i++ {
		instances = append(instances, &ec2.Instance{
			InstanceId: aws.String(fmt.Sprintf("%s-%d", "spot", i)),
			State: &ec2.InstanceState{
				Name: aws.String("running"),
			},
			InstanceLifecycle: aws.String("spot"),
		})
	}

	reservation := &ec2.Reservation{Instances: instances}
	reservations := []*ec2.Reservation{reservation}
	awsClient.EXPECT().DescribeInstances(gomock.Any()).Times(1).Return(
		&ec2.DescribeInstancesOutput{
			Reservations: reservations,
		},
		nil)
}

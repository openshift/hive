package remotemachineset

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
)

func TestAWSActuator(t *testing.T) {
	tests := []struct {
		name                         string
		mockAWSClient                func(*mockaws.MockClient)
		clusterDeployment            *hivev1.ClusterDeployment
		poolName                     string
		existing                     []runtime.Object
		expectedMachineSetReplicas   map[string]int64
		expectedSubnetIDInMachineSet bool
		expectedErr                  bool
		expectedCondition            *hivev1.MachinePoolCondition
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				testMachinePool(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
		},
		{
			name:              "generate machinesets across zones",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				testMachinePool(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1", "zone2", "zone3"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified zones",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
					return pool
				}(),
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "generate machinesets for specified zones and subnets",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
					pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "subnet-zone3"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2", "zone3"}, []string{"subnet-zone1", "subnet-zone2", "subnet-zone3"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
			expectedSubnetIDInMachineSet: true,
		},
		{
			name:              "list zones returns zero",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				testMachinePool(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, nil)
			},
			expectedErr: true,
		},
		{
			name:              "subnets specfied in the machinepool do not exist",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
					pool.Spec.Platform.AWS.Subnets = []string{"missing-subnet1", "missing-subnet2", "missing-subnet3"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeMissingSubnets(client, []string{"missing-subnet1", "missing-subnet2", "missing-subnet3"})
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "SubnetsNotFound",
			},
		},
		{
			name:              "more than one subnet for availability zone",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
					pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "subnet-zone3"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone1", "zone2"}, []string{"subnet-zone1", "subnet-zone2", "subnet-zone3"})
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "MoreThanOneSubnetForZone",
			},
		},
		{
			name:              "no subnet for availability zone",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
					pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2", "zone3"}, []string{"subnet-zone1", "subnet-zone2"})
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "NoSubnetForAvailabilityZone",
			},
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			fakeClient := fake.NewFakeClient(test.existing...)
			awsClient := mockaws.NewMockClient(mockCtrl)

			// set up mock expectations
			if test.mockAWSClient != nil {
				test.mockAWSClient(awsClient)
			}

			actuator := &AWSActuator{
				client:    fakeClient,
				awsClient: awsClient,
				logger:    log.WithField("actuator", "awsactuator"),
				region:    testRegion,
				amiID:     testAMI,
			}

			pool := &hivev1.MachinePool{}
			err := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: test.poolName}, pool)
			require.NoError(t, err)

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, pool, actuator.logger)
			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateAWSMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedSubnetIDInMachineSet)
			}
			if test.expectedCondition != nil {
				for _, cond := range pool.Status.Conditions {
					assert.Equal(t, cond.Type, test.expectedCondition.Type)
					assert.Equal(t, cond.Status, test.expectedCondition.Status)
					assert.Equal(t, cond.Reason, test.expectedCondition.Reason)
				}
			} else {
				// Assuming if you didn't expect a condition, there shouldn't be any.
				assert.Equal(t, 0, len(pool.Status.Conditions))
			}
		})
	}
}

func TestGetAWSAMIID(t *testing.T) {
	cases := []struct {
		name        string
		machineSets []machineapi.MachineSet
		expectError bool
	}{
		{
			name:        "no machinesets",
			expectError: true,
		},
		{
			name: "valid machineset",
			machineSets: []machineapi.MachineSet{
				*testMachineSet("ms1", "worker", true, 1, 0),
			},
		},
		{
			name: "invalid machineset",
			machineSets: []machineapi.MachineSet{
				func() machineapi.MachineSet {
					ms := testMachineSet("ms1", "worker", true, 1, 0)
					ms.Spec.Template.Spec.ProviderSpec.Value = nil
					return *ms
				}(),
			},
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			machineapi.SchemeBuilder.AddToScheme(scheme)
			awsprovider.SchemeBuilder.AddToScheme(scheme)
			actualAMIID, actualErr := getAWSAMIID(tc.machineSets, scheme, log.StandardLogger())
			if tc.expectError {
				assert.Error(t, actualErr, "expected an error")
			} else {
				if assert.NoError(t, actualErr, "unexpected error") {
					assert.Equal(t, testAMI, actualAMIID, "unexpected AMI ID")
				}
			}
		})
	}
}

func validateAWSMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64, expectedSubnetID bool) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		awsProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)
		assert.True(t, ok, "failed to convert to AWSMachineProviderConfig")

		assert.Equal(t, testInstanceType, awsProvider.InstanceType, "unexpected instance type")

		if assert.NotNil(t, awsProvider.AMI.ID, "missing AMI ID") {
			assert.Equal(t, testAMI, *awsProvider.AMI.ID, "unexpected AMI ID")
		}

		if expectedSubnetID {
			providerConfig := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)
			assert.NotNil(t, providerConfig.Subnet.ID, "missing subnet ID")
			assert.Equal(t, "subnet-"+providerConfig.Placement.AvailabilityZone, *providerConfig.Subnet.ID, "unexpected subnet ID")
		}
	}
}

func mockDescribeAvailabilityZones(client *mockaws.MockClient, zones []string) {
	input := &ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{{
			Name:   pointer.StringPtr("region-name"),
			Values: []*string{pointer.StringPtr(testRegion)},
		}},
	}
	availabilityZones := make([]*ec2.AvailabilityZone, len(zones))
	for i := range zones {
		availabilityZones[i] = &ec2.AvailabilityZone{
			ZoneName: &zones[i],
		}
	}
	output := &ec2.DescribeAvailabilityZonesOutput{
		AvailabilityZones: availabilityZones,
	}
	client.EXPECT().DescribeAvailabilityZones(input).Return(output, nil)
}

func mockDescribeSubnets(client *mockaws.MockClient, zones []string, subnetIDs []string) {
	idPointers := make([]*string, 0, len(subnetIDs))
	for _, id := range subnetIDs {
		idPointers = append(idPointers, aws.String(id))
	}
	input := &ec2.DescribeSubnetsInput{
		SubnetIds: idPointers,
	}
	subnets := make([]*ec2.Subnet, len(subnetIDs))
	for i := range subnetIDs {
		subnets[i] = &ec2.Subnet{
			SubnetId:         &subnetIDs[i],
			AvailabilityZone: &zones[i],
		}
	}
	output := &ec2.DescribeSubnetsOutput{
		Subnets: subnets,
	}
	client.EXPECT().DescribeSubnets(input).Return(output, nil)
}

func mockDescribeMissingSubnets(client *mockaws.MockClient, subnetIDs []string) {
	idPointers := make([]*string, 0, len(subnetIDs))
	for _, id := range subnetIDs {
		idPointers = append(idPointers, aws.String(id))
	}
	input := &ec2.DescribeSubnetsInput{
		SubnetIds: idPointers,
	}
	client.EXPECT().DescribeSubnets(input).Return(nil, fmt.Errorf("InvalidSubnets"))
}

func generateAWSMachineSetName(zone string) string {
	return fmt.Sprintf("%s-%s-%s", testInfraID, testPoolName, zone)
}

func encodeAWSMachineProviderSpec(awsProviderSpec *awsprovider.AWSMachineProviderConfig, scheme *runtime.Scheme) (*runtime.RawExtension, error) {
	serializer := jsonserializer.NewSerializer(jsonserializer.DefaultMetaFactory, scheme, scheme, false)
	var buffer bytes.Buffer
	err := serializer.Encode(awsProviderSpec, &buffer)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: buffer.Bytes(),
	}, nil
}

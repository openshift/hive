package remotemachineset

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
)

func TestAWSActuator(t *testing.T) {
	tests := []struct {
		name                       string
		mockAWSClient              func(*mockaws.MockClient)
		clusterDeployment          *hivev1.ClusterDeployment
		pool                       *hivev1.MachinePool
		expectedMachineSetReplicas map[string]int64
		expectedErr                bool
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testClusterDeployment(),
			pool:              testMachinePool(),
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
			pool:              testMachinePool(),
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
			pool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
		},
		{
			name:              "list zones returns zero",
			clusterDeployment: testClusterDeployment(),
			pool:              testMachinePool(),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, nil)
			},
			expectedErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			awsClient := mockaws.NewMockClient(mockCtrl)

			// set up mock expectations
			if test.mockAWSClient != nil {
				test.mockAWSClient(awsClient)
			}

			actuator := &AWSActuator{
				client: awsClient,
				logger: log.WithField("actuator", "awsactuator"),
				region: testRegion,
				amiID:  testAMI,
			}

			generatedMachineSets, err := actuator.GenerateMachineSets(test.clusterDeployment, test.pool, actuator.logger)

			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateAWSMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas)
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

func validateAWSMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64) {
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

package machinepool

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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsprovider/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awshivev1 "github.com/openshift/hive/apis/hive/v1/aws"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	fakeKMSKeyARN = "fakearn"
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
		expectedKMSKey               string
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
					pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "subnet-zone3",
						"pubSubnet-zone1", "pubSubnet-zone2", "pubSubnet-zone3"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2", "zone3"},
					[]string{"subnet-zone1", "subnet-zone2", "subnet-zone3"},
					[]string{"pubSubnet-zone1", "pubSubnet-zone2", "pubSubnet-zone3"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1":    false,
					"subnet-zone2":    false,
					"subnet-zone3":    false,
					"pubSubnet-zone1": true,
					"pubSubnet-zone2": true,
					"pubSubnet-zone3": true,
				}, "vpc-1")
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
			name:              "more than one private subnet for availability zone",
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
				mockDescribeSubnets(client, []string{"zone1", "zone1", "zone2"},
					[]string{"subnet-zone1", "subnet-zone2", "subnet-zone3"}, []string{}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1": false,
					"subnet-zone2": false,
					"subnet-zone3": false,
				}, "vpc-1")
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "MoreThanOneSubnetForZone",
			},
		},
		{
			name:              "no private subnet for availability zone",
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
				mockDescribeSubnets(client, []string{"zone1", "zone2", "zone3"},
					[]string{"subnet-zone1", "subnet-zone2"}, []string{}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1": false,
					"subnet-zone2": false,
				}, "vpc-1")
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "NoSubnetForAvailabilityZone",
			},
		},
		{
			name:              "no public subnet for availability zone and private subnet",
			clusterDeployment: testClusterDeployment(),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() *hivev1.MachinePool {
					pool := testMachinePool()
					pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
					pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1"}
					return pool
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2"},
					[]string{"subnet-zone1", "subnet-zone2"}, []string{"pubSubnet-zone1"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1":    false,
					"subnet-zone2":    false,
					"pubSubnet-zone1": true,
				}, "vpc-1")
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "InsufficientPublicSubnets",
			},
		},
		{
			name:              "supported spot market options",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.5.0"),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				withSpotMarketOptions(testMachinePool()),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
		},
		{
			name:              "unsupported spot market options",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.4.0"),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				withSpotMarketOptions(testMachinePool()),
			},
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "UnsupportedSpotMarketOptions",
			},
		},
		{
			name:              "kms key disk encryption",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.5.0"),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				withKMSKey(testMachinePool()),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedKMSKey: fakeKMSKeyARN,
		},
		{
			name:              "unsupported configuration condition cleared",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.4.0"),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				func() runtime.Object {
					mp := testMachinePool()
					mp.Status.Conditions = []hivev1.MachinePoolCondition{{
						Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
						Status: corev1.ConditionTrue,
					}}
					return mp
				}(),
			},
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionFalse,
				Reason: "ConfigurationSupported",
			},
		},
		{
			name:              "malformed cluster version",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "bad-version"),
			poolName:          testMachinePool().Name,
			existing: []runtime.Object{
				withSpotMarketOptions(testMachinePool()),
			},
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "UnsupportedSpotMarketOptions",
			},
		},
		{
			name: "generate single CAPI machineset for single zone",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.MachineManagement = &hivev1.MachineManagement{
					Central: &hivev1.CentralMachineManagement{},
				}
				return cd
			}(),
			poolName: testMachinePool().Name,
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
			name: "generate CAPI machinesets for specified zones",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				cd.Spec.MachineManagement = &hivev1.MachineManagement{
					Central: &hivev1.CentralMachineManagement{},
				}
				return cd
			}(),
			poolName: testMachinePool().Name,
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

			cd := test.clusterDeployment
			if cd.Spec.Platform.AWS != nil && cd.Spec.MachineManagement != nil && cd.Spec.MachineManagement.Central != nil {
				generatedMachineSets, _, err := actuator.GenerateCAPIMachineSets(cd, pool, actuator.logger)
				if test.expectedErr {
					assert.Error(t, err, "expected error for test case")
				} else {
					validateAWSCAPIMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedSubnetIDInMachineSet, test.expectedKMSKey)
				}
			} else {
				generatedMachineSets, _, err := actuator.GenerateMachineSets(cd, pool, actuator.logger)
				if test.expectedErr {
					assert.Error(t, err, "expected error for test case")
				} else {
					validateAWSMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedSubnetIDInMachineSet, test.expectedKMSKey)
				}
			}

			if test.expectedCondition != nil {
				cond := controllerutils.FindMachinePoolCondition(pool.Status.Conditions, test.expectedCondition.Type)
				if assert.NotNilf(t, cond, "did not find expected condition type: %v", test.expectedCondition.Type) {
					assert.Equal(t, test.expectedCondition.Status, cond.Status, "condition found with unexpected status")
					assert.Equal(t, test.expectedCondition.Reason, cond.Reason, "condition found with unexpected reason")
				}
			}
		})
	}
}

func TestGetAWSAMIID(t *testing.T) {
	cases := []struct {
		name          string
		masterMachine *machineapi.Machine
		expectError   bool
	}{
		{
			name:          "valid master machine",
			masterMachine: testMachine("master1", "master"),
		},
		{
			name: "invalid master machine",
			masterMachine: func() *machineapi.Machine {
				ms := testMachine("master1", "master")
				ms.Spec.ProviderSpec.Value = nil
				return ms
			}(),
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			machineapi.SchemeBuilder.AddToScheme(scheme)
			awsprovider.SchemeBuilder.AddToScheme(scheme)
			actualAMIID, actualErr := getAWSAMIID(tc.masterMachine, scheme, log.StandardLogger())
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

func validateAWSMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64, expectedSubnetID bool, expectedKMSKey string) {
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

		assert.Equal(t, expectedKMSKey, *awsProvider.BlockDevices[0].EBS.KMSKey.ARN)

		if expectedSubnetID {
			providerConfig := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)
			assert.NotNil(t, providerConfig.Subnet.ID, "missing subnet ID")
			assert.Equal(t, "subnet-"+providerConfig.Placement.AvailabilityZone, *providerConfig.Subnet.ID, "unexpected subnet ID")
		}
	}
}

func validateAWSCAPIMachineSets(t *testing.T, mSets []*capiv1.MachineSet, expectedMSReplicas map[string]int64, expectedSubnetID bool, expectedKMSKey string) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set") {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch")
		}

		// TODO: Validate AWSMachineTemplate for CAPI machineSet
		/*
			awsProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)
			assert.True(t, ok, "failed to convert to AWSMachineProviderConfig")

			assert.Equal(t, testInstanceType, awsProvider.InstanceType, "unexpected instance type")

			if assert.NotNil(t, awsProvider.AMI.ID, "missing AMI ID") {
				assert.Equal(t, testAMI, *awsProvider.AMI.ID, "unexpected AMI ID")
			}

			assert.Equal(t, expectedKMSKey, *awsProvider.BlockDevices[0].EBS.KMSKey.ARN)

			if expectedSubnetID {
				providerConfig := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)
				assert.NotNil(t, providerConfig.Subnet.ID, "missing subnet ID")
				assert.Equal(t, "subnet-"+providerConfig.Placement.AvailabilityZone, *providerConfig.Subnet.ID, "unexpected subnet ID")
			}
		*/
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

func mockDescribeSubnets(client *mockaws.MockClient, zones []string, privateSubnetIDs []string, pubSubnetIDs []string, vpcID string) {
	idPointers := make([]*string, 0, len(privateSubnetIDs)+len(pubSubnetIDs))
	for _, id := range privateSubnetIDs {
		idPointers = append(idPointers, aws.String(id))
	}
	for _, id := range pubSubnetIDs {
		idPointers = append(idPointers, aws.String(id))
	}
	input := &ec2.DescribeSubnetsInput{
		SubnetIds: idPointers,
	}
	subnets := make([]*ec2.Subnet, len(privateSubnetIDs)+len(pubSubnetIDs))
	for i := range privateSubnetIDs {
		subnets[i] = &ec2.Subnet{
			SubnetId:         &privateSubnetIDs[i],
			AvailabilityZone: &zones[i],
			VpcId:            &vpcID,
		}
	}
	for i := range pubSubnetIDs {
		subnets[len(privateSubnetIDs)+i] = &ec2.Subnet{
			SubnetId:         &pubSubnetIDs[i],
			AvailabilityZone: &zones[i],
			VpcId:            &vpcID,
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

func mockDescribeRouteTables(client *mockaws.MockClient, subnets map[string]bool, vpc string) {
	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("vpc-id"),
			Values: []*string{aws.String(vpc)},
		}},
	}

	output := &ec2.DescribeRouteTablesOutput{
		RouteTables: constructRouteTables(subnets),
	}

	client.EXPECT().DescribeRouteTables(input).Return(output, nil)
}

// Takes a list of subnets with bool indicating if the corresponding subnet is public
func constructRouteTables(subnets map[string]bool) (routeTablesOut []*ec2.RouteTable) {
	routeTablesOut = append(routeTablesOut,
		&ec2.RouteTable{
			Associations: []*ec2.RouteTableAssociation{{Main: aws.Bool(true)}},
			Routes: []*ec2.Route{{
				DestinationCidrBlock: aws.String("0.0.0.0/0"),
				GatewayId:            aws.String("igw-main"),
			}},
		})

	for subnetID := range subnets {
		routeTablesOut = append(
			routeTablesOut,
			constructRouteTable(
				subnetID,
				subnets[subnetID],
			),
		)
	}
	return
}

func constructRouteTable(subnetID string, public bool) *ec2.RouteTable {
	var gatewayID string
	if public {
		gatewayID = "igw-" + subnetID[len(subnetID)-8:8]
	} else {
		gatewayID = "vgw-" + subnetID[len(subnetID)-8:8]
	}
	return &ec2.RouteTable{
		Associations: []*ec2.RouteTableAssociation{{SubnetId: aws.String(subnetID)}},
		Routes: []*ec2.Route{{
			DestinationCidrBlock: aws.String("0.0.0.0/0"),
			GatewayId:            aws.String(gatewayID),
		}},
	}
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

func withSpotMarketOptions(pool *hivev1.MachinePool) *hivev1.MachinePool {
	pool.Spec.Platform.AWS.SpotMarketOptions = &awshivev1.SpotMarketOptions{}
	return pool
}

func withKMSKey(pool *hivev1.MachinePool) *hivev1.MachinePool {
	pool.Spec.Platform.AWS.EC2RootVolume.KMSKeyARN = fakeKMSKeyARN
	return pool
}

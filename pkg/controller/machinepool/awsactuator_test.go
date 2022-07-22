package machinepool

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	machineapi "github.com/openshift/api/machine/v1beta1"

	"github.com/openshift/hive/apis"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awshivev1 "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	"github.com/openshift/hive/pkg/constants"
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
		machinePool                  *hivev1.MachinePool
		masterMachine                *machineapi.Machine
		expectedMachineSetReplicas   map[string]int64
		expectedSubnetIDInMachineSet bool
		expectedErr                  bool
		expectedCondition            *hivev1.MachinePoolCondition
		expectedKMSKey               string
		expectedAMI                  *machineapi.AWSResourceReference
		expectedSGFilters            []machineapi.Filter
	}{
		{
			name:              "generate single machineset for single zone",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "generate machinesets across zones",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1", "zone2", "zone3"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "generate machinesets for specified zones",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "generate machinesets for specified zones and subnets",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "subnet-zone3",
					"pubSubnet-zone1", "pubSubnet-zone2", "pubSubnet-zone3"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
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
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "list zones returns zero",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, nil)
			},
			expectedErr: true,
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "subnets specfied in the machinepool do not exist",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				pool.Spec.Platform.AWS.Subnets = []string{"missing-subnet1", "missing-subnet2", "missing-subnet3"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeMissingSubnets(client, []string{"missing-subnet1", "missing-subnet2", "missing-subnet3"})
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "SubnetsNotFound",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "more than one private subnet for availability zone",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "subnet-zone3"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
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
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "no private subnet for availability zone",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2", "zone3"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
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
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "no public subnet for availability zone and private subnet",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
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
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "public subnets all don't have route tables pointing to igw",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1", "pubSubnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2"},
					[]string{"subnet-zone1", "subnet-zone2"}, []string{"pubSubnet-zone1", "pubSubnet-zone2"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1":    false,
					"subnet-zone2":    false,
					"pubSubnet-zone1": false,
					"pubSubnet-zone2": false,
				}, "vpc-1")
			},
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionFalse,
				Reason: "ValidSubnets",
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 2,
				generateAWSMachineSetName("zone2"): 1,
			},
			expectedSubnetIDInMachineSet: true,
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "public subnets some don't have route tables pointing to igw",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1", "pubSubnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2"},
					[]string{"subnet-zone1", "subnet-zone2"}, []string{"pubSubnet-zone1", "pubSubnet-zone2"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"subnet-zone1":    false,
					"subnet-zone2":    false,
					"pubSubnet-zone1": true,
					"pubSubnet-zone2": false,
				}, "vpc-1")
			},
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionFalse,
				Reason: "ValidSubnets",
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 2,
				generateAWSMachineSetName("zone2"): 1,
			},
			expectedSubnetIDInMachineSet: true,
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			// Public subnets may be used exclusively when privatelink is enabled for ClusterDeployment.
			name: "only public subnets (tagged as public 'kubernetes.io/role/elb' in AWS, no internet gateway), privateLink enabled",
			clusterDeployment: func() *hivev1.ClusterDeployment {
				cd := testClusterDeployment()
				// Privatelink is enabled for the ClusterDeployment.
				cd.Spec.Platform.AWS.PrivateLink = &awshivev1.PrivateLinkAccess{
					Enabled: true,
				}
				return cd
			}(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				// MachinePool has only public subnets.
				pool.Spec.Platform.AWS.Subnets = []string{"pubSubnet-zone1", "pubSubnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				// AWS API returns the configured public subnets. Each public subnet returned will be tagged 'kubernetes.io/role/elb'.
				mockDescribeSubnets(client, []string{"zone1", "zone2"},
					[]string{}, []string{"pubSubnet-zone1", "pubSubnet-zone2"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"pubSubnet-zone1": false,
					"pubSubnet-zone2": false,
				}, "vpc-1")
			},
			// We expect this to be a valid configuration.
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionFalse,
				Reason: "ValidSubnets",
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 2,
				generateAWSMachineSetName("zone2"): 1,
			},
			// MachineSets will have an explicit subnet ID.
			// TODO: HIVE-1805: Uncomment the line below when HIVE-1805 has been addressed,
			// allowing public subnets to be configured for machinepools when privatelink is enabled.
			//expectedSubnetIDInMachineSet: true,
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "supported spot market options",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.5.0"),
			machinePool:       withSpotMarketOptions(testMachinePool()),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "unsupported spot market options",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.4.0"),
			machinePool:       withSpotMarketOptions(testMachinePool()),
			masterMachine:     testMachine("master0", "master"),
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "UnsupportedSpotMarketOptions",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "kms key disk encryption",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.5.0"),
			machinePool:       withKMSKey(testMachinePool()),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedKMSKey: fakeKMSKeyARN,
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "unsupported configuration condition cleared",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.4.0"),
			machinePool: func() *hivev1.MachinePool {
				mp := testMachinePool()
				mp.Status.Conditions = []hivev1.MachinePoolCondition{{
					Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
					Status: corev1.ConditionTrue,
				}}
				return mp
			}(),
			masterMachine: testMachine("master0", "master"),
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
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "malformed cluster version",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "bad-version"),
			machinePool:       withSpotMarketOptions(testMachinePool()),
			masterMachine:     testMachine("master0", "master"),
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "UnsupportedSpotMarketOptions",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
		},
		{
			name:              "master machine references AMI by tag",
			clusterDeployment: testClusterDeployment(),
			machinePool:       testMachinePool(),
			masterMachine: func() *machineapi.Machine {
				m := testMachine("master0", "master")
				awsMachineProviderConfig := testAWSProviderSpec()
				awsMachineProviderConfig.AMI = machineapi.AWSResourceReference{
					Filters: []machineapi.Filter{
						{
							Name: "tag:Name",
							Values: []string{
								fmt.Sprintf("%s-ami-%s",
									testClusterDeployment().Spec.ClusterMetadata.InfraID,
									testClusterDeployment().Spec.Platform.AWS.Region),
							},
						},
					},
				}
				m.Spec.ProviderSpec.Value, _ = encodeAWSMachineProviderSpec(awsMachineProviderConfig, scheme.Scheme)
				return m
			}(),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1", "zone2", "zone3"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 1,
				generateAWSMachineSetName("zone2"): 1,
				generateAWSMachineSetName("zone3"): 1,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				Filters: []machineapi.Filter{
					{
						Name: "tag:Name",
						Values: []string{
							fmt.Sprintf("%s-ami-%s",
								testClusterDeployment().Spec.ClusterMetadata.InfraID,
								testClusterDeployment().Spec.Platform.AWS.Region),
						},
					},
				},
			},
		},
		{
			name:              "machinepool has an ExtraWorkerSecurityGroup annotation",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Annotations = map[string]string{
					constants.ExtraWorkerSecurityGroupAnnotation: "extra-security-group",
				}
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1", "pubSubnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				gomock.InOrder(
					mockDescribeSubnets(client, []string{"zone1", "zone2"},
						[]string{"subnet-zone1", "subnet-zone2"}, []string{"pubSubnet-zone1", "pubSubnet-zone2"}, "vpc-1"),
					// When an ExtraWorkerSecurityGroup annotation is configured we query the subnets for the first subnet
					// in the list configured for a MachinePool to discover the VPC ID of the subnet.
					mockDescribeSubnets(client, []string{"zone1"},
						[]string{"subnet-zone1"}, []string{}, "vpc-1"),
				)
				mockDescribeRouteTables(client,
					map[string]bool{
						"subnet-zone1":    false,
						"subnet-zone2":    false,
						"pubSubnet-zone1": true,
						"pubSubnet-zone2": true,
					},
					"vpc-1")
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 2,
				generateAWSMachineSetName("zone2"): 1,
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.StringPtr(testAMI),
			},
			expectedSGFilters: []machineapi.Filter{
				{
					Name:   "tag:Name",
					Values: []string{"foo-12345-worker-sg", "extra-security-group"},
				},
				{
					Name:   "vpc-id",
					Values: []string{"vpc-1"},
				},
			},
		},
		{
			name:              "machinepool has an ExtraWorkerSecurityGroup annotation but has no subnets",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Annotations = map[string]string{
					constants.ExtraWorkerSecurityGroupAnnotation: "extra-security-group",
				}
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			expectedErr:   true,
		},
		{
			name:              "machinepool has an ExtraWorkerSecurityGroup annotation but no subnets found in AWS",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Annotations = map[string]string{
					constants.ExtraWorkerSecurityGroupAnnotation: "extra-security-group",
				}
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"subnet-zone1", "subnet-zone2", "pubSubnet-zone1", "pubSubnet-zone2"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				client.EXPECT().DescribeSubnets(gomock.Any()).Return(&ec2.DescribeSubnetsOutput{Subnets: []*ec2.Subnet{}}, errors.New("not found"))
			},
			expectedErr: true,
		},
	}

	for _, test := range tests {
		apis.AddToScheme(scheme.Scheme)
		machineapi.AddToScheme(scheme.Scheme)

		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			fakeClient := fake.NewFakeClient([]runtime.Object{
				test.machinePool,
			}...)
			awsClient := mockaws.NewMockClient(mockCtrl)

			// set up mock expectations
			if test.mockAWSClient != nil {
				test.mockAWSClient(awsClient)
			}

			logger := log.WithFields(log.Fields{"machinePool": test.machinePool.Name})
			actuator, err := NewAWSActuator(fakeClient, awsclient.CredentialsSource{}, test.clusterDeployment.Spec.Platform.AWS.Region, test.machinePool, test.masterMachine, scheme.Scheme, logger)
			require.NoError(t, err)
			actuator.awsClient = awsClient

			pool := &hivev1.MachinePool{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: test.machinePool.Name}, pool)
			require.NoError(t, err)

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, pool, actuator.logger)
			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateAWSMachineSets(t, generatedMachineSets, test.expectedMachineSetReplicas, test.expectedSubnetIDInMachineSet, test.expectedKMSKey, test.expectedAMI, test.expectedSGFilters)
			}
			if test.expectedCondition != nil {
				cond := controllerutils.FindCondition(pool.Status.Conditions, test.expectedCondition.Type)
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
			machineapi.AddToScheme(scheme)
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

func validateAWSMachineSets(t *testing.T, mSets []*machineapi.MachineSet, expectedMSReplicas map[string]int64, expectedSubnetID bool, expectedKMSKey string, expectedAMI *machineapi.AWSResourceReference, expectedSGFilters []machineapi.Filter) {
	assert.Equal(t, len(expectedMSReplicas), len(mSets), "different number of machine sets generated than expected")

	for _, ms := range mSets {
		expectedReplicas, ok := expectedMSReplicas[ms.Name]
		if assert.True(t, ok, "unexpected machine set", ms.Name) {
			assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch", ms.Name)
		}

		awsProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AWSMachineProviderConfig)
		assert.True(t, ok, "failed to convert to AWSMachineProviderConfig")

		assert.Equal(t, testInstanceType, awsProvider.InstanceType, "unexpected instance type")

		if assert.NotNil(t, awsProvider.AMI, "missing AMI") {
			if expectedAMI.ID != nil {
				assert.Equal(t, *expectedAMI.ID, *awsProvider.AMI.ID, "unexpected AMI ID")
			}
			if len(expectedAMI.Filters) > 0 {
				assert.Equal(t, expectedAMI.Filters, awsProvider.AMI.Filters, "unexpected AMI filter")
			}
		}

		assert.Equal(t, expectedKMSKey, *awsProvider.BlockDevices[0].EBS.KMSKey.ARN)

		if expectedSubnetID {
			providerConfig := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AWSMachineProviderConfig)
			if assert.NotNil(t, providerConfig.Subnet.ID, "missing subnet ID") {
				assert.Equal(t, "subnet-"+providerConfig.Placement.AvailabilityZone, *providerConfig.Subnet.ID, "unexpected subnet ID")
			}
		}
		if expectedSGFilters != nil {
			assert.Equal(t, awsProvider.SecurityGroups[0].Filters, expectedSGFilters, "unexpected security group filters")
		}
	}
}

func mockDescribeAvailabilityZones(client *mockaws.MockClient, zones []string) *gomock.Call {
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
	return client.EXPECT().DescribeAvailabilityZones(input).Return(output, nil)
}

func mockDescribeSubnets(client *mockaws.MockClient, zones []string, privateSubnetIDs []string, pubSubnetIDs []string, vpcID string) *gomock.Call {
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
			Tags: []*ec2.Tag{{
				Key:   aws.String(tagNameSubnetPublicELB),
				Value: aws.String("1"),
			}},
		}
	}
	output := &ec2.DescribeSubnetsOutput{
		Subnets: subnets,
	}
	return client.EXPECT().DescribeSubnets(input).Return(output, nil)
}

func mockDescribeMissingSubnets(client *mockaws.MockClient, subnetIDs []string) *gomock.Call {
	idPointers := make([]*string, 0, len(subnetIDs))
	for _, id := range subnetIDs {
		idPointers = append(idPointers, aws.String(id))
	}
	input := &ec2.DescribeSubnetsInput{
		SubnetIds: idPointers,
	}
	return client.EXPECT().DescribeSubnets(input).Return(nil, fmt.Errorf("InvalidSubnets"))
}

func mockDescribeRouteTables(client *mockaws.MockClient, subnets map[string]bool, vpc string) *gomock.Call {
	input := &ec2.DescribeRouteTablesInput{
		Filters: []*ec2.Filter{{
			Name:   aws.String("vpc-id"),
			Values: []*string{aws.String(vpc)},
		}},
	}

	output := &ec2.DescribeRouteTablesOutput{
		RouteTables: constructRouteTables(subnets),
	}

	return client.EXPECT().DescribeRouteTables(input).Return(output, nil)
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

func encodeAWSMachineProviderSpec(awsProviderSpec *machineapi.AWSMachineProviderConfig, scheme *runtime.Scheme) (*runtime.RawExtension, error) {
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

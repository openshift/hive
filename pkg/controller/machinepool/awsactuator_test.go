package machinepool

import (
	"bytes"
	"context"
	"encoding/json"
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
	"k8s.io/utils/pointer"

	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	awshivev1 "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	mockaws "github.com/openshift/hive/pkg/awsclient/mock"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	fakeKMSKeyARN = "fakearn"
)

var (
	baseUserTagsFromCD = map[string]string{
		"cd-label":              "cd-value",
		"cd-label-sync-from-mp": "cd-value-2",
	}
)

func TestAWSActuator(t *testing.T) {
	type ttype struct {
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
		expectedEC2MetadataAuth      string
		expectedAMI                  *machineapi.AWSResourceReference
		expectedSGFilters            []machineapi.Filter
		// Security groups identified by ID start with the *second* SecurityGroup ([1])
		// and do not use Filters.
		expectedSGIDs    []string
		expectedUserTags map[string]string
	}
	tests := []ttype{
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "MoreThanOneSubnetForZone",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
		},
		{
			// This test exists to hit the NotEnoughSubnetsForZones error path. Because of our simple
			// preflight checks (number of subnets has to be 0, numZones, or 2*numZones) the only way
			// we can do it is by giving 2*numZones subnets, but an uneven number of each. Specifically,
			// since public subnets are checked first in this case, we have to give more private than
			// public; otherwise we'll catch MoreThanOneSubnetForZone as above.
			name:              "improper mix of public and private subnets",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.Zones = []string{"zone1", "zone2"}
				pool.Spec.Platform.AWS.Subnets = []string{"priv1-zone1", "priv2-zone2", "priv3-zone2", "pub-zone1"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeSubnets(client, []string{"zone1", "zone2", "zone2", "zone1"},
					[]string{"priv1-zone1", "priv2-zone2", "priv3-zone2"}, []string{"pub-zone1"}, "vpc-1")
				mockDescribeRouteTables(client, map[string]bool{
					"priv1-zone1": false,
					"priv2-zone2": false,
					"priv3-zone2": false,
					"pub-zone1":   true,
				}, "vpc-1")
			},
			expectedErr: true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "NotEnoughSubnetsForZones",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
			expectedErr:   true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "WrongNumberOfSubnets",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
			expectedErr:   true,
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.InvalidSubnetsMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "WrongNumberOfSubnets",
			},
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
		},
		{
			name:              "ec2 metadata",
			clusterDeployment: withClusterVersion(testClusterDeployment(), "4.5.0"),
			machinePool:       withEC2Metadata(testMachinePool(), "Optional"),
			masterMachine:     testMachine("master0", "master"),
			mockAWSClient: func(client *mockaws.MockClient) {
				mockDescribeAvailabilityZones(client, []string{"zone1"})
			},
			expectedMachineSetReplicas: map[string]int64{
				generateAWSMachineSetName("zone1"): 3,
			},
			expectedEC2MetadataAuth: "Optional",
			expectedAMI: &machineapi.AWSResourceReference{
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: baseUserTagsFromCD,
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
				m.Spec.ProviderSpec.Value, _ = encodeAWSMachineProviderSpec(awsMachineProviderConfig, scheme.GetScheme())
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
			expectedUserTags: baseUserTagsFromCD,
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
				ID: pointer.String(testAMI),
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
			expectedUserTags: baseUserTagsFromCD,
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
		{
			name:              "machinepool has AdditionalSecurityGroupIDs",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.AdditionalSecurityGroupIDs = []string{
					"sg-one", "sg-two",
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
				ID: pointer.String(testAMI),
			},
			expectedSGFilters: []machineapi.Filter{
				{
					Name:   "tag:Name",
					Values: []string{"foo-12345-worker-sg"},
				},
			},
			expectedSGIDs:    []string{"sg-one", "sg-two"},
			expectedUserTags: baseUserTagsFromCD,
		},
		{
			name:              "ExtraWorkerSecurityGroup and AdditionalSecurityGroupIDs are mutually exclusive",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Annotations = map[string]string{
					constants.ExtraWorkerSecurityGroupAnnotation: "extra-security-group",
				}
				pool.Spec.Platform.AWS.AdditionalSecurityGroupIDs = []string{"additional-security-group"}
				return pool
			}(),
			masterMachine: testMachine("master0", "master"),
			expectedCondition: &hivev1.MachinePoolCondition{
				Type:   hivev1.UnsupportedConfigurationMachinePoolCondition,
				Status: corev1.ConditionTrue,
				Reason: "SecurityGroupOptionConflict",
			},
			expectedUserTags: baseUserTagsFromCD,
		},
		{
			name:              "machinepool has UserTags",
			clusterDeployment: testClusterDeployment(),
			machinePool: func() *hivev1.MachinePool {
				pool := testMachinePool()
				pool.Spec.Platform.AWS.UserTags = map[string]string{
					"pool-label":            "pool-value",
					"pool-label-2":          "pool-value-2",
					"cd-label-sync-from-mp": "pool-precedence-value",
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
				ID: pointer.String(testAMI),
			},
			expectedUserTags: map[string]string{
				"pool-label":            "pool-value",
				"pool-label-2":          "pool-value-2",
				"cd-label":              "cd-value",
				"cd-label-sync-from-mp": "pool-precedence-value",
			},
		},
	}

	validateAWSMachineSets := func(t *testing.T, mSets []*machineapi.MachineSet, test ttype) {
		assert.Equal(t, len(test.expectedMachineSetReplicas), len(mSets), "different number of machine sets generated than expected")

		for _, ms := range mSets {
			expectedReplicas, ok := test.expectedMachineSetReplicas[ms.Name]
			if assert.True(t, ok, "unexpected machine set", ms.Name) {
				assert.Equal(t, expectedReplicas, int64(*ms.Spec.Replicas), "replica mismatch", ms.Name)
			}

			awsProvider, ok := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AWSMachineProviderConfig)
			assert.True(t, ok, "failed to convert to AWSMachineProviderConfig")

			assert.Equal(t, testInstanceType, awsProvider.InstanceType, "unexpected instance type")

			if assert.NotNil(t, awsProvider.AMI, "missing AMI") {
				if test.expectedAMI.ID != nil {
					assert.Equal(t, *test.expectedAMI.ID, *awsProvider.AMI.ID, "unexpected AMI ID")
				}
				if len(test.expectedAMI.Filters) > 0 {
					assert.Equal(t, test.expectedAMI.Filters, awsProvider.AMI.Filters, "unexpected AMI filter")
				}
			}

			assert.Equal(t, test.expectedKMSKey, *awsProvider.BlockDevices[0].EBS.KMSKey.ARN)

			if test.expectedSubnetIDInMachineSet {
				providerConfig := ms.Spec.Template.Spec.ProviderSpec.Value.Object.(*machineapi.AWSMachineProviderConfig)
				assert.NotNil(t, providerConfig.Subnet.ID, "missing subnet ID")
				assert.Equal(t, "subnet-"+providerConfig.Placement.AvailabilityZone, *providerConfig.Subnet.ID, "unexpected subnet ID")
			}
			if test.expectedSGFilters != nil {
				assert.Equal(t, awsProvider.SecurityGroups[0].Filters, test.expectedSGFilters, "unexpected security group filters")
			}

			if test.expectedSGIDs != nil {
				if assert.Equal(t, len(test.expectedSGIDs), len(awsProvider.SecurityGroups)-1, "expected n-1 SecurityGroups holding IDs") {
					for i := 0; i < len(test.expectedSGIDs); i++ {
						assert.Equal(t, test.expectedSGIDs[i], *awsProvider.SecurityGroups[i+1].ID, "mismatched security group ID")
					}
				}
			}
			if test.expectedEC2MetadataAuth != "" {
				assert.NotNil(t, awsProvider.MetadataServiceOptions, "Missing ec2metadata")
				assert.Equal(t, test.expectedEC2MetadataAuth, string(awsProvider.MetadataServiceOptions.Authentication))
			}
			if test.expectedUserTags != nil {
				if assert.Equal(t, len(test.expectedUserTags), len(awsProvider.Tags)-1, "expected n-1 user tags") {
					for _, tag := range awsProvider.Tags {
						value, ok := test.expectedUserTags[tag.Name]
						// skip owner tag, no need to match non user tags
						if tag.Name == "kubernetes.io/cluster/foo-12345" {
							continue
						}
						assert.True(t, ok, fmt.Sprintf("mismatched key '%s' of user tags", tag.Name))
						assert.Equal(t, value, tag.Value, "mismatched value of user tags")
					}
				}
			}
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			mockCtrl := gomock.NewController(t)
			scheme := scheme.GetScheme()
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.machinePool).Build()
			awsClient := mockaws.NewMockClient(mockCtrl)

			// set up mock expectations
			if test.mockAWSClient != nil {
				test.mockAWSClient(awsClient)
			}

			logger := log.WithFields(log.Fields{"machinePool": test.machinePool.Name})
			actuator, err := NewAWSActuator(fakeClient, awsclient.CredentialsSource{}, test.clusterDeployment.Spec.Platform.AWS.Region, test.machinePool, test.masterMachine, scheme, logger)
			require.NoError(t, err)
			actuator.awsClient = awsClient

			pool := &hivev1.MachinePool{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: testNamespace, Name: test.machinePool.Name}, pool)
			require.NoError(t, err)

			generatedMachineSets, _, err := actuator.GenerateMachineSets(test.clusterDeployment, pool, actuator.logger)
			if test.expectedErr {
				assert.Error(t, err, "expected error for test case")
			} else {
				validateAWSMachineSets(t, generatedMachineSets, test)
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
			scheme := scheme.GetScheme()
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

func mockDescribeAvailabilityZones(client *mockaws.MockClient, zones []string) *gomock.Call {
	input := &ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{{
			Name:   pointer.String("region-name"),
			Values: []*string{pointer.String(testRegion)},
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
	var buf1, buf2 bytes.Buffer
	if err := serializer.Encode(awsProviderSpec, &buf1); err != nil {
		return nil, err
	}
	// HACK: Trim whitespace, which the fake client seems to do internally, resulting in technically-unequal values
	// and spurious updates.
	if err := json.Compact(&buf2, buf1.Bytes()); err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: buf2.Bytes(),
	}, nil
}

func withEC2Metadata(pool *hivev1.MachinePool, metadataAuth string) *hivev1.MachinePool {
	pool.Spec.Platform.AWS.EC2Metadata = &awshivev1.EC2Metadata{Authentication: "Optional"}
	return pool
}

func withKMSKey(pool *hivev1.MachinePool) *hivev1.MachinePool {
	pool.Spec.Platform.AWS.EC2RootVolume.KMSKeyARN = fakeKMSKeyARN
	return pool
}

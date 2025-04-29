package awsprivatelink

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/davecgh/go-spew/spew"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/awsclient/mock"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNS = "test-namespace"
)

func Test_setErrCondition(t *testing.T) {
	scheme := scheme.GetScheme()
	cases := []struct {
		name string

		conditions []hivev1.ClusterDeploymentCondition
		err        error
		reason     string

		expectedConditions []hivev1.ClusterDeploymentCondition
	}{{
		name: "no previous failure",

		err:    errors.New("failed to do something important"),
		reason: "FailureToDoSomethingImportant",

		expectedConditions: getExpectedConditions(true, "FailureToDoSomethingImportant",
			"failed to do something important"),
	}, {
		name: "previous failure",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPrevious",
			Message: "failed to do something important previously",
		}},
		err:    errors.New("failed to do something important"),
		reason: "FailureToDoSomethingImportant",

		expectedConditions: getExpectedConditions(true, "FailureToDoSomethingImportant",
			"failed to do something important"),
	}, {
		name: "previous ready",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "AllLookingGood",
			Message: "all is looking good",
		}},
		err:    errors.New("failed to do something important"),
		reason: "FailureToDoSomethingImportant",

		expectedConditions: getExpectedConditions(true, "FailureToDoSomethingImportant",
			"failed to do something important"),
	}, {
		name: "previous failure, ready",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}, {
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}},
		err:    errors.New("failed to do something important"),
		reason: "FailureToDoSomethingImportant",

		expectedConditions: getExpectedConditions(true, "FailureToDoSomethingImportant",
			"failed to do something important"),
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cd := testcd.FullBuilder(testNS, "test", scheme).Build()
			cd.Status.Conditions = test.conditions

			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(cd).Build()
			reconciler := &ReconcileAWSPrivateLink{
				Client: fakeClient,
			}
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			err := reconciler.setErrCondition(cd, test.reason, test.err, logger)
			require.NoError(t, err)

			cd = &hivev1.ClusterDeployment{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: testNS, Name: "test"}, cd)
			require.NoError(t, err)

			// santize
			for i := range cd.Status.Conditions {
				cd.Status.Conditions[i].LastProbeTime = metav1.Time{}
				cd.Status.Conditions[i].LastTransitionTime = metav1.Time{}
			}

			assert.ElementsMatch(t, test.expectedConditions, cd.Status.Conditions)
		})
	}
}

func Test_setProgressCondition(t *testing.T) {
	scheme := scheme.GetScheme()
	cases := []struct {
		name string

		conditions []hivev1.ClusterDeploymentCondition
		completed  corev1.ConditionStatus
		message    string
		reason     string

		expectedConditions []hivev1.ClusterDeploymentCondition
	}{{
		name: "no previous progress, not completed",

		completed: corev1.ConditionFalse,
		message:   "progresing towards stage 1",
		reason:    "InprogesStage1",
		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage1",
			Message: "progresing towards stage 1",
		}},
	}, {
		name: "previous progress, not completed",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage1",
			Message: "progresing towards stage 1",
		}},
		completed: corev1.ConditionFalse,
		message:   "progresing towards stage 2",
		reason:    "InprogesStage2",

		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage2",
			Message: "progresing towards stage 2",
		}},
	}, {
		name: "previous failure, progress no completed",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}},
		completed: corev1.ConditionFalse,
		message:   "progresing towards stage 1",
		reason:    "InprogesStage1",

		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}, {
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage1",
			Message: "progresing towards stage 1",
		}},
	}, {
		name: "previous failure, previous progress no completed",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}, {
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage1",
			Message: "progresing towards stage 1",
		}},
		completed: corev1.ConditionFalse,
		message:   "progresing towards stage 2",
		reason:    "InprogesStage2",

		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}, {
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage2",
			Message: "progresing towards stage 2",
		}},
	}, {
		name: "previous failure, progress completed",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  "FailureToDoSomethingImportantPreviously",
			Message: "failed to do something important previously",
		}, {
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "InprogesStage1",
			Message: "progresing towards stage 1",
		}},
		completed: corev1.ConditionTrue,
		message:   "All looking good",
		reason:    "AllLookingGood",

		expectedConditions: getExpectedConditions(false, "AllLookingGood",
			"All looking good"),
	}, {
		name: "previous ready, now progressing",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "AllLookingGood",
			Message: "All looking good",
		}},
		completed: corev1.ConditionFalse,
		message:   "progresing towards stage 1",
		reason:    "InprogesStage1",

		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "AllLookingGood",
			Message: "All looking good",
		}},
	}, {
		name: "previous ready, now ready with different reason",

		conditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "AllLookingGood",
			Message: "All looking good",
		}},
		completed: corev1.ConditionTrue,
		message:   "All looking good",
		reason:    "AllLookingGoodVersion2",

		expectedConditions: getExpectedConditions(false, "AllLookingGoodVersion2",
			"All looking good"),
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cd := testcd.FullBuilder(testNS, "test", scheme).Build()
			cd.Status.Conditions = test.conditions

			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(cd).Build()
			reconciler := &ReconcileAWSPrivateLink{
				Client: fakeClient,
			}
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			err := reconciler.setReadyCondition(cd, test.completed, test.reason, test.message, logger)
			require.NoError(t, err)

			cd = &hivev1.ClusterDeployment{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Namespace: testNS, Name: "test"}, cd)
			require.NoError(t, err)

			// santize
			for i := range cd.Status.Conditions {
				cd.Status.Conditions[i].LastProbeTime = metav1.Time{}
				cd.Status.Conditions[i].LastTransitionTime = metav1.Time{}
			}

			assert.ElementsMatch(t, test.expectedConditions, cd.Status.Conditions)
		})
	}
}

func TestInitialURL(t *testing.T) {
	tests := []struct {
		name string

		existing map[string]string

		want string
	}{{
		name: "use kubeconfig",

		existing: map[string]string{
			"kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://api.test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin
`,
		},
		want: "api.test-cluster",
	}, {
		name: "use raw-kubeconfig",

		existing: map[string]string{
			"raw-kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://api.test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin
`,
		},
		want: "api.test-cluster",
	}, {
		name: "use raw-kubeconfig when both present",

		existing: map[string]string{
			"raw-kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://api.test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin
`,
			"kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://api.test-cluster:6443
  name: test-cluster
- cluster:
    server: https://api.vanity-domain:6443
  name: test-cluster-vanity
contexts:
- context:
    cluster: test-cluster-vanity
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin
`,
		},
		want: "api.test-cluster",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testSecret("test", tt.existing)
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(s).Build()

			got, err := initialURL(fakeClient, client.ObjectKey{Namespace: testNS, Name: "test"})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func testSecret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNS,
			Name:      name,
		},
		Data: map[string][]byte{},
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

// createVpcEndpointInputMatcher implements gomock.Matcher, succeeding if the input is an
// ec2.CreateVpcEndpointInput whose VpcId equals the configured vpcId.
type createVpcEndpointInputMatcher struct {
	vpcId string
}

func (m createVpcEndpointInputMatcher) Matches(o interface{}) bool {
	i, ok := o.(*ec2.CreateVpcEndpointInput)
	if !ok {
		return false
	}
	return *i.VpcId == m.vpcId
}

func (m createVpcEndpointInputMatcher) String() string {
	return fmt.Sprintf("has VPC ID %s", m.vpcId)
}

func TestReconcile(t *testing.T) {
	scheme := scheme.GetScheme()

	key := client.ObjectKey{Name: "test-cd", Namespace: testNS}
	cdBuilder := testcd.FullBuilder(testNS, "test-cd", scheme)
	enabledPrivateLinkBuilder := cdBuilder.
		Options(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
			PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			}),
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Status: corev1.ConditionUnknown,
				Type:   hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			}),
		)
	validInventory := []hivev1.AWSPrivateLinkInventory{{
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			Region: "us-east-1",
			VPCID:  "vpc-1",
		},
		Subnets: []hivev1.AWSPrivateLinkSubnet{{
			AvailabilityZone: "us-east-1a",
			SubnetID:         "subnet-1",
		}, {
			AvailabilityZone: "us-east-1b",
			SubnetID:         "subnet-2",
		}, {
			AvailabilityZone: "us-east-1c",
			SubnetID:         "subnet-3",
		}},
	}}
	kubeConfigSecret := map[string]string{
		"kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://api.test-cluster:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin`,
	}

	mockDiscoverLB := func(m *mock.MockClient) string {
		clusternlb := &elbv2.LoadBalancer{
			LoadBalancerArn: aws.String("aws:elb:12345:nlb-arn"),
			State: &elbv2.LoadBalancerState{
				Code: aws.String(elbv2.LoadBalancerStateEnumActive),
			},
		}
		m.EXPECT().DescribeLoadBalancers(gomock.Any()).
			Return(&elbv2.DescribeLoadBalancersOutput{
				LoadBalancers: []*elbv2.LoadBalancer{clusternlb},
			}, nil).AnyTimes()
		return *clusternlb.LoadBalancerArn
	}

	mockCreateService := func(m *mock.MockClient, clusternlb string) *ec2.ServiceConfiguration {
		service := &ec2.ServiceConfiguration{
			AcceptanceRequired:      aws.Bool(false),
			ServiceId:               aws.String("vpce-svc-12345"),
			ServiceName:             aws.String("vpce-svc-12345.vpc.amazon.com"),
			ServiceState:            aws.String(ec2.ServiceStateAvailable),
			NetworkLoadBalancerArns: aws.StringSlice([]string{clusternlb}),
			AvailabilityZones:       aws.StringSlice([]string{"us-east-1b", "us-east-1c"}),
		}
		m.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
			Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{}, nil)
		m.EXPECT().CreateVpcEndpointServiceConfiguration(gomock.Any()).
			Return(&ec2.CreateVpcEndpointServiceConfigurationOutput{
				ServiceConfiguration: service,
			}, nil)
		m.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
			Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
				ServiceConfigurations: []*ec2.ServiceConfiguration{service},
			}, nil)
		return service
	}
	mockServicePerms := func(m *mock.MockClient, service *ec2.ServiceConfiguration) {
		m.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{Arn: aws.String("aws:iam:12345:hub-user")}, nil)
		m.EXPECT().DescribeVpcEndpointServicePermissions(gomock.Any()).
			Return(&ec2.DescribeVpcEndpointServicePermissionsOutput{}, nil)
		m.EXPECT().ModifyVpcEndpointServicePermissions(&ec2.ModifyVpcEndpointServicePermissionsInput{
			AddAllowedPrincipals: aws.StringSlice([]string{"aws:iam:12345:hub-user"}),
			ServiceId:            service.ServiceId,
		}).Return(nil, nil)
	}
	mockExistingService := func(m *mock.MockClient, clusternlb string, modify func(*ec2.ServiceConfiguration)) *ec2.ServiceConfiguration {
		service := &ec2.ServiceConfiguration{
			AcceptanceRequired:      aws.Bool(false),
			ServiceId:               aws.String("vpce-svc-12345"),
			ServiceName:             aws.String("vpce-svc-12345.vpc.amazon.com"),
			ServiceState:            aws.String(ec2.ServiceStateAvailable),
			NetworkLoadBalancerArns: aws.StringSlice([]string{clusternlb}),
		}
		modify(service)
		m.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
			Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
				ServiceConfigurations: []*ec2.ServiceConfiguration{service},
			}, nil)
		return service
	}

	mockCreateEndpoint := func(m *mock.MockClient, service *ec2.ServiceConfiguration) *ec2.VpcEndpoint {
		m.EXPECT().DescribeVpcEndpoints(
			&ec2.DescribeVpcEndpointsInput{
				Filters: []*ec2.Filter{{
					Name:   aws.String("tag:hive.openshift.io/private-link-access-for"),
					Values: aws.StringSlice([]string{"test-cd-1234"}),
				}}}).
			Return(&ec2.DescribeVpcEndpointsOutput{}, nil)
		m.EXPECT().DescribeVpcEndpointsPages(
			&ec2.DescribeVpcEndpointsInput{
				Filters: []*ec2.Filter{{
					Name:   aws.String("vpc-id"),
					Values: aws.StringSlice([]string{"vpc-1"}),
				}}}, gomock.Any()).
			Do(func(input *ec2.DescribeVpcEndpointsInput, fn func(*ec2.DescribeVpcEndpointsOutput, bool) bool) {
				describeVpcEndpointsOutput := &ec2.DescribeVpcEndpointsOutput{}
				fn(describeVpcEndpointsOutput, true)
			})
		m.EXPECT().DescribeVpcEndpointServices(&ec2.DescribeVpcEndpointServicesInput{
			ServiceNames: aws.StringSlice([]string{*service.ServiceName}),
		}).Return(&ec2.DescribeVpcEndpointServicesOutput{
			ServiceDetails: []*ec2.ServiceDetail{{AvailabilityZones: service.AvailabilityZones}},
		}, nil)

		endpoint := &ec2.VpcEndpoint{
			VpcEndpointId: aws.String("vpce-12345"),
			VpcId:         aws.String("vpc-1"),
			State:         aws.String("available"),
			DnsEntries: []*ec2.DnsEntry{{
				DnsName:      aws.String("vpce-12345-us-east-1.vpce-svc-12345.vpc.amazonaws.com"),
				HostedZoneId: aws.String("HZ23456"),
			}},
		}
		m.EXPECT().CreateVpcEndpoint(gomock.Any()).
			Return(&ec2.CreateVpcEndpointOutput{VpcEndpoint: endpoint}, nil)
		m.EXPECT().DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
			VpcEndpointIds: aws.StringSlice([]string{*endpoint.VpcEndpointId}),
		}).Return(&ec2.DescribeVpcEndpointsOutput{
			VpcEndpoints: []*ec2.VpcEndpoint{endpoint},
		}, nil)
		return endpoint
	}

	mockPHZ := func(m *mock.MockClient, endpoint *ec2.VpcEndpoint, apiDomain string, existingSummary *route53.HostedZoneSummary) string {
		byVPCOut := &route53.ListHostedZonesByVPCOutput{}
		if existingSummary != nil {
			byVPCOut.HostedZoneSummaries = []*route53.HostedZoneSummary{existingSummary}
		}
		m.EXPECT().ListHostedZonesByVPC(&route53.ListHostedZonesByVPCInput{
			MaxItems:  aws.String("100"),
			VPCId:     endpoint.VpcId,
			VPCRegion: aws.String("us-east-1"),
		}).Return(byVPCOut, nil)
		var hzID string
		if existingSummary == nil {
			hzID = "HZ12345"
			m.EXPECT().CreateHostedZone(newCreateHostedZoneInputMatcher(&route53.CreateHostedZoneInput{
				HostedZoneConfig: &route53.HostedZoneConfig{
					PrivateZone: aws.Bool(true),
				},
				Name: aws.String(apiDomain),
				VPC: &route53.VPC{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				},
			})).Return(&route53.CreateHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
			}, nil)
		} else {
			hzID = aws.StringValue(existingSummary.HostedZoneId)
		}

		m.EXPECT().ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						AliasTarget: &route53.AliasTarget{
							DNSName:              endpoint.DnsEntries[0].DnsName,
							EvaluateTargetHealth: aws.Bool(false),
							HostedZoneId:         endpoint.DnsEntries[0].HostedZoneId,
						},
						Name: aws.String(apiDomain),
						Type: aws.String("A"),
					},
				}},
			},
			HostedZoneId: aws.String(hzID),
		})
		return hzID
	}

	mockPHZARecords := func(m *mock.MockClient, endpoint *ec2.VpcEndpoint, apiDomain string, existingSummary *route53.HostedZoneSummary, knownENIs map[string]string) string {
		byVPCOut := &route53.ListHostedZonesByVPCOutput{}
		if existingSummary != nil {
			byVPCOut.HostedZoneSummaries = []*route53.HostedZoneSummary{existingSummary}
		}
		m.EXPECT().ListHostedZonesByVPC(&route53.ListHostedZonesByVPCInput{
			MaxItems:  aws.String("100"),
			VPCId:     endpoint.VpcId,
			VPCRegion: aws.String("us-east-1"),
		}).Return(byVPCOut, nil)
		var hzID string
		if existingSummary == nil {
			hzID = "HZ12345"
			m.EXPECT().CreateHostedZone(newCreateHostedZoneInputMatcher(&route53.CreateHostedZoneInput{
				HostedZoneConfig: &route53.HostedZoneConfig{
					PrivateZone: aws.Bool(true),
				},
				Name: aws.String(apiDomain),
				VPC: &route53.VPC{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				},
			})).Return(&route53.CreateHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
			}, nil)
		} else {
			hzID = aws.StringValue(existingSummary.HostedZoneId)
		}

		eniReq := &ec2.DescribeNetworkInterfacesInput{
			NetworkInterfaceIds: endpoint.NetworkInterfaceIds,
		}
		eniResp := &ec2.DescribeNetworkInterfacesOutput{}
		var rr []*route53.ResourceRecord
		for _, eni := range aws.StringValueSlice(eniReq.NetworkInterfaceIds) {
			ip, ok := knownENIs[eni]
			if ok {
				eniResp.NetworkInterfaces = append(eniResp.NetworkInterfaces, &ec2.NetworkInterface{PrivateIpAddress: aws.String(ip)})
				rr = append(rr, &route53.ResourceRecord{Value: aws.String(ip)})
			}
		}
		m.EXPECT().DescribeNetworkInterfaces(eniReq).Return(eniResp, nil)

		sort.Slice(rr, func(i, j int) bool {
			return *rr[i].Value < *rr[j].Value
		})
		m.EXPECT().ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			ChangeBatch: &route53.ChangeBatch{
				Changes: []*route53.Change{{
					Action: aws.String("UPSERT"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name:            aws.String(apiDomain),
						Type:            aws.String("A"),
						TTL:             aws.Int64(10),
						ResourceRecords: rr,
					},
				}},
			},
			HostedZoneId: aws.String(hzID),
		})
		return hzID
	}

	cases := []struct {
		name string

		existing           []runtime.Object
		inventory          []hivev1.AWSPrivateLinkInventory
		associate          []hivev1.AWSAssociatedVPC
		dnsRecordType      hivev1.AWSPrivateLinkDNSRecordType
		configureAWSClient func(*mock.MockClient)

		hasFinalizer        bool
		expectDeleted       bool
		expectedAnnotations map[string]string
		expectedStatus      *hivev1aws.PrivateLinkAccessStatus
		expectedConditions  []hivev1.ClusterDeploymentCondition
		err                 string
	}{{
		name: "cd without initialized conditions",

		existing: []runtime.Object{
			cdBuilder.Build(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1"})),
		},
	}, {
		name: "cd with gcp platform",

		existing: []runtime.Object{
			cdBuilder.Options(func(cd *hivev1.ClusterDeployment) {
				cd.Spec.Platform.GCP = &hivev1gcp.Platform{Region: "gcp-region"}
			}).Build(),
		},
	}, {
		name: "cd without privatelink",

		existing: []runtime.Object{
			cdBuilder.Build(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1"})),
		},
	}, {
		name: "cd with privatelink disabled",

		existing: []runtime.Object{
			cdBuilder.Build(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: false}})),
		},
	}, {
		name: "cd with privatelink enabled, no inventory",

		existing: []runtime.Object{
			enabledPrivateLinkBuilder.Build(),
		},

		hasFinalizer: true,
		expectedConditions: getExpectedConditions(true, "UnsupportedRegion",
			"cluster deployment region \"us-east-1\" is not supported as there is no inventory to create necessary resources"),
	}, {
		name: "cd with privatelink enabled, no inventory in given region",

		existing: []runtime.Object{
			enabledPrivateLinkBuilder.Build(),
		},
		inventory: []hivev1.AWSPrivateLinkInventory{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				Region: "us-west-1",
				VPCID:  "vpc-1",
			},
		}},

		hasFinalizer: true,
		expectedConditions: getExpectedConditions(true, "UnsupportedRegion",
			"cluster deployment region \"us-east-1\" is not supported as there is no inventory to create necessary resources"),
	}, {
		name: "cd with privatelink enabled, no provision started",

		existing: []runtime.Object{
			enabledPrivateLinkBuilder.Build(),
		},
		inventory: validInventory,

		hasFinalizer: true,
	}, {
		name: "cd with privatelink enabled, provision started, but no cluster metadata",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0"),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,

		hasFinalizer: true,
	}, {
		name: "cd with privatelink enabled, provision started, but no admin kubeconfig",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0", provisionWithInfraID("test-cd-1234")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,

		hasFinalizer: true,
	}, {
		name: "cd with privatelink enabled, provision started, nlb describe access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			m.EXPECT().DescribeLoadBalancers(gomock.Any()).
				Return(nil, awserr.New("AccessDenied", "not authorized to DescribeLoadBalancers", nil))
		},

		hasFinalizer: true,
		expectedConditions: getExpectedConditions(true, "DiscoveringNLBFailed",
			"failed to describe load balancer for the cluster: AccessDenied: not authorized to DescribeLoadBalancers"),
		err: "failed to describe load balancer for the cluster: AccessDenied: not authorized to DescribeLoadBalancers",
	}, {
		name: "cd with privatelink enabled, provision started, nlb not found",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			m.EXPECT().DescribeLoadBalancers(gomock.Any()).
				Return(nil, awserr.New("LoadBalancerNotFound", "Loadbalance could not be found", nil))
		},
		expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  "DiscoveringNLBNotYetFound",
			Message: "discovering NLB for the cluster, but it does not exist yet",
		}},

		hasFinalizer: true,
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)

			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
		},
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, previous service exists, acceptance required set to true, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockExistingService(m, clusternlb, func(s *ec2.ServiceConfiguration) {
				s.AcceptanceRequired = aws.Bool(true)
			})

			m.EXPECT().ModifyVpcEndpointServiceConfiguration(&ec2.ModifyVpcEndpointServiceConfigurationInput{
				ServiceId:          service.ServiceId,
				AcceptanceRequired: aws.Bool(false),
			}).Return(&ec2.ModifyVpcEndpointServiceConfigurationOutput{}, nil)

			mockServicePerms(m, service)

			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
		},
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, previous service exists, additional NLB added, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockExistingService(m, clusternlb, func(s *ec2.ServiceConfiguration) {
				s.NetworkLoadBalancerArns = aws.StringSlice([]string{clusternlb, "aws:elb:12345:not-cluster-nlb-arn"})
			})

			m.EXPECT().ModifyVpcEndpointServiceConfiguration(&ec2.ModifyVpcEndpointServiceConfigurationInput{
				ServiceId:                     service.ServiceId,
				AcceptanceRequired:            aws.Bool(false),
				RemoveNetworkLoadBalancerArns: aws.StringSlice([]string{"aws:elb:12345:not-cluster-nlb-arn"}),
			}).Return(&ec2.ModifyVpcEndpointServiceConfigurationOutput{}, nil)

			mockServicePerms(m, service)

			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
		},
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, previous service exists, permissions change, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockExistingService(m, clusternlb, func(s *ec2.ServiceConfiguration) {})

			m.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{Arn: aws.String("aws:iam:12345:hub-user")}, nil)
			m.EXPECT().DescribeVpcEndpointServicePermissions(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServicePermissionsOutput{
					AllowedPrincipals: []*ec2.AllowedPrincipal{{
						Principal: aws.String("aws:iam:12345:some-that-should-not-be-allowed"),
					}},
				}, nil)
			m.EXPECT().ModifyVpcEndpointServicePermissions(&ec2.ModifyVpcEndpointServicePermissionsInput{
				AddAllowedPrincipals:    aws.StringSlice([]string{"aws:iam:12345:hub-user"}),
				RemoveAllowedPrincipals: aws.StringSlice([]string{"aws:iam:12345:some-that-should-not-be-allowed"}),
				ServiceId:               service.ServiceId,
			}).Return(nil, nil)

			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
		},
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, previous service exists, permissions change add additional allowed principals, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			func() *hivev1.ClusterDeployment {
				// Need to instantiate a new builder for each test otherwise the enabledPrivateLinkBuilder defined earlier assumes
				// the modifications for all tests that use the predefined builder function.
				// eg. using a function like withAdditionalAllowedPrincipals(...) to modify:
				// enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0"),withAdditionalAllowedPrincipals(...)),
				cd := cdBuilder.
					Options(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
						PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
						testcd.WithCondition(hivev1.ClusterDeploymentCondition{
							Status: corev1.ConditionUnknown,
							Type:   hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
						}),
						testcd.WithCondition(hivev1.ClusterDeploymentCondition{
							Status: corev1.ConditionUnknown,
							Type:   hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
						}),
					).Build()
				cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: "test-cd-provision-0"}
				cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals = &[]string{"aws:iam:12345:another-user", "aws:iam:12345:some-user"}
				return cd
			}(),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockExistingService(m, clusternlb, func(s *ec2.ServiceConfiguration) {})

			m.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{Arn: aws.String("aws:iam:12345:hub-user")}, nil)
			m.EXPECT().DescribeVpcEndpointServicePermissions(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServicePermissionsOutput{
					AllowedPrincipals: []*ec2.AllowedPrincipal{{
						Principal: aws.String("aws:iam:12345:some-that-should-not-be-allowed"),
					}},
				}, nil)
			m.EXPECT().ModifyVpcEndpointServicePermissions(&ec2.ModifyVpcEndpointServicePermissionsInput{
				AddAllowedPrincipals:    aws.StringSlice([]string{"aws:iam:12345:another-user", "aws:iam:12345:hub-user", "aws:iam:12345:some-user"}),
				RemoveAllowedPrincipals: aws.StringSlice([]string{"aws:iam:12345:some-that-should-not-be-allowed"}),
				ServiceId:               service.ServiceId,
			}).Return(nil, nil)
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{
				Name:                        "vpce-svc-12345.vpc.amazon.com",
				ID:                          "vpce-svc-12345",
				DefaultAllowedPrincipal:     aws.String("aws:iam:12345:hub-user"),
				AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:another-user", "aws:iam:12345:some-user"},
			},
		},
		// AdditionalAllowedPrincipals are effected on the VPC Endpoint Service before we DescribeVpcEndpoints so fail to describe them
		// to simplify the test.
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, previous service exists, permissions change remove additional allowed principals, endpoint access denied",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			func() *hivev1.ClusterDeployment {
				// Need to instantiate a new builder for each test otherwise the enabledPrivateLinkBuilder defined earlier assumes
				// the modifications for all tests that use the predefined builder function.
				// eg. using a function like withAdditionalAllowedPrincipals(...) to modify:
				// enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0"),withAdditionalAllowedPrincipals(...)),
				cd := cdBuilder.
					Options(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
						PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
						testcd.WithCondition(hivev1.ClusterDeploymentCondition{
							Status: corev1.ConditionUnknown,
							Type:   hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
						}),
						testcd.WithCondition(hivev1.ClusterDeploymentCondition{
							Status: corev1.ConditionUnknown,
							Type:   hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
						}),
					).Build()
				cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: "test-cd-provision-0"}
				// There are no additionalAllowedPrincipals defined
				cd.Spec.Platform.AWS.PrivateLink.AdditionalAllowedPrincipals = nil
				initPrivateLinkStatus(cd)
				// Status has previous additionalAllowedPrincipals defined
				cd.Status.Platform.AWS.PrivateLink.VPCEndpointService.AdditionalAllowedPrincipals = &[]string{"aws:iam:12345:another-user", "aws:iam:12345:some-user"}
				return cd
			}(),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockExistingService(m, clusternlb, func(s *ec2.ServiceConfiguration) {})

			m.EXPECT().GetCallerIdentity(gomock.Any()).Return(&sts.GetCallerIdentityOutput{Arn: aws.String("aws:iam:12345:hub-user")}, nil)
			m.EXPECT().DescribeVpcEndpointServicePermissions(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServicePermissionsOutput{
					AllowedPrincipals: []*ec2.AllowedPrincipal{
						{Principal: aws.String("aws:iam:12345:another-user")},
						{Principal: aws.String("aws:iam:12345:hub-user")},
						{Principal: aws.String("aws:iam:12345:some-user")},
					},
				}, nil)
			m.EXPECT().ModifyVpcEndpointServicePermissions(&ec2.ModifyVpcEndpointServicePermissionsInput{
				RemoveAllowedPrincipals: aws.StringSlice([]string{"aws:iam:12345:another-user", "aws:iam:12345:some-user"}),
				ServiceId:               service.ServiceId,
			}).Return(nil, nil)
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awserr.New("AccessDenied", "not authorized to DescribeVpcEndpoints", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{
				Name:                    "vpce-svc-12345.vpc.amazon.com",
				ID:                      "vpce-svc-12345",
				DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user"),
				// There will be no additionalAllowedPrincipals recorded in status
			},
		},
		// AdditionalAllowedPrincipals are effected on the VPC Endpoint Service before we DescribeVpcEndpoints so fail to describe them
		// to simplify the test.
		expectedConditions: getExpectedConditions(true, "VPCEndpointReconcileFailed",
			"AccessDenied: not authorized to DescribeVpcEndpoints"),
		err: "failed to reconcile the VPC Endpoint: AccessDenied: not authorized to DescribeVpcEndpoints",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, no matching az",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: []hivev1.AWSPrivateLinkInventory{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				Region: "us-east-1",
				VPCID:  "vpc-1",
			},
		}},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)

			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{}, nil)
			m.EXPECT().DescribeVpcEndpointServices(&ec2.DescribeVpcEndpointServicesInput{
				ServiceNames: aws.StringSlice([]string{*service.ServiceName}),
			}).Return(&ec2.DescribeVpcEndpointServicesOutput{
				ServiceDetails: []*ec2.ServiceDetail{{AvailabilityZones: service.AvailabilityZones}},
			}, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
		},
		expectedConditions: getExpectedConditions(true, "NoSupportedAZsInInventory",
			"no supported VPC in inventory which support the AZs of the service"),
		err: "failed to reconcile the VPC Endpoint: no supported VPC in inventory which support the AZs of the service",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, validate spread",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		// An inventory with more than one VPC
		inventory: []hivev1.AWSPrivateLinkInventory{
			{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
					Region: "us-east-1",
					VPCID:  "vpc-1",
				},
				Subnets: []hivev1.AWSPrivateLinkSubnet{{
					AvailabilityZone: "us-east-1a",
					SubnetID:         "subnet-1",
				}, {
					AvailabilityZone: "us-east-1b",
					SubnetID:         "subnet-2",
				}, {
					AvailabilityZone: "us-east-1c",
					SubnetID:         "subnet-3",
				}},
			},
			{
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
					Region: "us-east-1",
					VPCID:  "vpc-2",
				},
				Subnets: []hivev1.AWSPrivateLinkSubnet{{
					AvailabilityZone: "us-east-1a",
					SubnetID:         "subnet-4",
				}, {
					AvailabilityZone: "us-east-1b",
					SubnetID:         "subnet-5",
				}, {
					AvailabilityZone: "us-east-1c",
					SubnetID:         "subnet-6",
				}},
			},
		},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)

			// Query endpoints for the cluster. There are none, triggering the create path.
			m.EXPECT().DescribeVpcEndpoints(
				&ec2.DescribeVpcEndpointsInput{
					Filters: []*ec2.Filter{{
						Name:   aws.String("tag:hive.openshift.io/private-link-access-for"),
						Values: aws.StringSlice([]string{"test-cd-1234"}),
					}}}).
				Return(&ec2.DescribeVpcEndpointsOutput{}, nil)
			m.EXPECT().DescribeVpcEndpointServices(&ec2.DescribeVpcEndpointServicesInput{
				ServiceNames: aws.StringSlice([]string{*service.ServiceName}),
			}).Return(&ec2.DescribeVpcEndpointServicesOutput{
				ServiceDetails: []*ec2.ServiceDetail{{AvailabilityZones: service.AvailabilityZones}},
			}, nil)

			// Query all endpoints for the VPCs in the inventory. There are two in vpc-1 and one in vpc-2.
			m.EXPECT().DescribeVpcEndpointsPages(
				&ec2.DescribeVpcEndpointsInput{
					Filters: []*ec2.Filter{{
						Name:   aws.String("vpc-id"),
						Values: aws.StringSlice([]string{"vpc-1", "vpc-2"}),
					}}}, gomock.Any()).
				Do(func(input *ec2.DescribeVpcEndpointsInput, fn func(*ec2.DescribeVpcEndpointsOutput, bool) bool) {
					describeVpcEndpointsOutput := &ec2.DescribeVpcEndpointsOutput{
						VpcEndpoints: []*ec2.VpcEndpoint{
							{
								VpcEndpointId: aws.String("vpce-11"),
								VpcId:         aws.String("vpc-1"),
							},
							{
								VpcEndpointId: aws.String("vpce-12"),
								VpcId:         aws.String("vpc-1"),
							},
							{
								VpcEndpointId: aws.String("vpce-21"),
								VpcId:         aws.String("vpc-2"),
							},
						},
					}
					fn(describeVpcEndpointsOutput, true)
				})

			createdEndpoint := &ec2.VpcEndpoint{
				VpcEndpointId: aws.String("vpce-22"),
				VpcId:         aws.String("vpc-2"),
				State:         aws.String("available"),
				DnsEntries: []*ec2.DnsEntry{{
					DnsName:      aws.String("vpce-22-us-east-1.vpce-svc-12345.vpc.amazonaws.com"),
					HostedZoneId: aws.String("HZ22"),
				}},
			}

			// This is the crux of the test. Expect to be asked to create the endpoint in vpc-2 due
			// to "spread" logic, since vpc-2 was emptier.
			m.EXPECT().CreateVpcEndpoint(createVpcEndpointInputMatcher{"vpc-2"}).
				Return(&ec2.CreateVpcEndpointOutput{VpcEndpoint: createdEndpoint}, nil)

			m.EXPECT().DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{
				VpcEndpointIds: aws.StringSlice([]string{*createdEndpoint.VpcEndpointId}),
			}).Return(&ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []*ec2.VpcEndpoint{createdEndpoint},
			}, nil)

			hzID := mockPHZ(m, createdEndpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ22"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     createdEndpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)

		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-22",
			HostedZoneID:       "HZ22",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, no kubeconfig secret",

		existing: []runtime.Object{
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			mockCreateEndpoint(m, service)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
		},
		expectedConditions: getExpectedConditions(true, "CouldNotCalculateAPIDomain",
			"secrets \"test-cd-provision-0-kubeconfig\" not found"),
		err: "secrets \"test-cd-provision-0-kubeconfig\" not found",
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, no previous PHZ",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", nil)

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, no previous PHZ (A records)",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory:     validInventory,
		dnsRecordType: hivev1.ARecordAWSPrivateLinkDNSRecordType,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			knownENI := map[string]string{
				"eni-1": "ip-1",
				"eni-2": "ip-2",
				"eni-3": "ip-3",
			}
			endpoint.NetworkInterfaceIds = aws.StringSlice([]string{"eni-2", "eni-1"})

			hzID := mockPHZARecords(m, endpoint, "api.test-cluster", nil, knownENI)

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, existing PHZ, no record for endpoint",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, provision started, nlb found, no previous service, no previous endpoint, existing PHZ, no record for endpoint (A records)",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory:     validInventory,
		dnsRecordType: hivev1.ARecordAWSPrivateLinkDNSRecordType,
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			knownENI := map[string]string{
				"eni-1": "ip-1",
				"eni-2": "ip-2",
				"eni-3": "ip-3",
			}
			endpoint.NetworkInterfaceIds = aws.StringSlice([]string{"eni-2", "eni-1"})

			hzID := mockPHZARecords(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			}, knownENI)

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, no previous private link, associate vpcs fails",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		associate: []hivev1.AWSAssociatedVPC{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-hive1",
				Region: "us-west-1",
			},
		}},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)

			m.EXPECT().AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, awserr.New("AccessDenied", "AssociateVPCWithHostedZone access denied", nil))
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(true, "AssociatingVPCsToHostedZoneFailed",
			"AccessDenied: AssociateVPCWithHostedZone access denied"),
		err: "AccessDenied: AssociateVPCWithHostedZone access denied",
	}, {
		name: "cd with privatelink enabled, no previous private link, associate vpcs",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		associate: []hivev1.AWSAssociatedVPC{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-hive1",
				Region: "us-west-1",
			},
		}},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)

			m.EXPECT().AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, no previous private link, associate vpcs remove some previous ones",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		associate: []hivev1.AWSAssociatedVPC{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-hive1",
				Region: "us-west-1",
			},
		}},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}, {
					VPCId:     aws.String("vpc-hive1-removed"),
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)

			m.EXPECT().AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, nil)
			m.EXPECT().DisassociateVPCFromHostedZone(&route53.DisassociateVPCFromHostedZoneInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1-removed"),
					VPCRegion: aws.String("us-east-1"),
				},
			}).Return(nil, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, no previous private link, associate vpcs across accounts",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig")),
			enabledPrivateLinkBuilder.Build(withClusterProvision("test-cd-provision-0")),
		},
		inventory: validInventory,
		associate: []hivev1.AWSAssociatedVPC{{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-hive1",
				Region: "us-west-1",
			},
			CredentialsSecretRef: &corev1.LocalObjectReference{
				Name: "hivev1-creds",
			},
		}},
		configureAWSClient: func(m *mock.MockClient) {
			clusternlb := mockDiscoverLB(m)
			service := mockCreateService(m, clusternlb)
			mockServicePerms(m, service)
			endpoint := mockCreateEndpoint(m, service)

			hzID := mockPHZ(m, endpoint, "api.test-cluster", &route53.HostedZoneSummary{
				HostedZoneId: aws.String("HZ12345"),
				Name:         aws.String("api.test-cluster"),
			})

			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53.HostedZone{
					Id: aws.String(hzID),
				},
				VPCs: []*route53.VPC{{
					VPCId:     endpoint.VpcId,
					VPCRegion: aws.String("us-east-1"),
				}},
			}, nil)

			m.EXPECT().CreateVPCAssociationAuthorization(&route53.CreateVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, nil)
			m.EXPECT().AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, nil)
			m.EXPECT().DeleteVPCAssociationAuthorization(&route53.DeleteVPCAssociationAuthorizationInput{
				HostedZoneId: aws.String("HZ12345"),
				VPC: &route53.VPC{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: aws.String("us-west-1"),
				},
			}).Return(nil, nil)
		},

		hasFinalizer: true,
		expectedStatus: &hivev1aws.PrivateLinkAccessStatus{
			VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345", DefaultAllowedPrincipal: aws.String("aws:iam:12345:hub-user")},
			VPCEndpointID:      "vpce-12345",
			HostedZoneID:       "HZ12345",
		},
		expectedConditions: getExpectedConditions(false, "PrivateLinkAccessReady",
			"private link access is ready for use"),
	}, {
		name: "cd with privatelink enabled, previous provision failed, new started",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig"),
				provisionWithFailed()),
			testProvision("test-cd-provision-1",
				provisionWithPrevInfraID("test-cd-1234")),
			enabledPrivateLinkBuilder.Build(
				withClusterMetadata("test-cd-1234", "test-cd-provision-0-kubeconfig"),
				withClusterProvision("test-cd-provision-1"),
				withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345"},
					VPCEndpointID:      "vpce-12345",
					HostedZoneID:       "HZ12345",
				}),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
					Status:  corev1.ConditionTrue,
					Reason:  "PrivateLinkAccessReady",
					Message: "private link access is ready for use",
				}),
			),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
			rr := &route53.ResourceRecordSet{
				Type: aws.String("A"),
				Name: aws.String("api.test-cluster"),
				AliasTarget: &route53.AliasTarget{
					DNSName: aws.String("vpc.."),
				},
			}
			m.EXPECT().ListResourceRecordSets(&route53.ListResourceRecordSetsInput{
				HostedZoneId: aws.String("HZ12345"),
			}).Return(&route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []*route53.ResourceRecordSet{{
					Type: aws.String("NS"),
				}, {
					Type: aws.String("SOA"),
				}, rr},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
				HostedZoneId: aws.String("HZ12345"),
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{{
						Action:            aws.String("DELETE"),
						ResourceRecordSet: rr,
					}},
				},
			}).Return(nil, nil)
			m.EXPECT().DeleteHostedZone(&route53.DeleteHostedZoneInput{
				Id: aws.String("HZ12345"),
			}).Return(nil, nil)

			endpoint := &ec2.VpcEndpoint{
				VpcEndpointId: aws.String("vpce-12345"),
				VpcId:         aws.String("vpc-1"),
			}
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{{
						VpcEndpointId: endpoint.VpcEndpointId,
						VpcId:         endpoint.VpcId,
					}},
				}, nil).Times(1)
			m.EXPECT().DeleteVpcEndpoints(&ec2.DeleteVpcEndpointsInput{
				VpcEndpointIds: aws.StringSlice([]string{*endpoint.VpcEndpointId}),
			}).Return(nil, nil)

			m.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{{
						ServiceId: aws.String("vpce-svc-12345"),
					}},
				}, nil)
			m.EXPECT().DeleteVpcEndpointServiceConfigurations(&ec2.DeleteVpcEndpointServiceConfigurationsInput{
				ServiceIds: aws.StringSlice([]string{"vpce-svc-12345"}),
			}).Return(nil, nil)
		},

		hasFinalizer: true,
		expectedAnnotations: map[string]string{
			lastCleanupAnnotationKey: "test-cd-1234",
		}, expectedConditions: []hivev1.ClusterDeploymentCondition{{
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Status:  corev1.ConditionTrue,
			Reason:  "PrivateLinkAccessReady",
			Message: "private link access is ready for use",
		}},
	}, {
		name: "cd with privatelink enabled, previous provision failed, new started, cleanup already done",

		existing: []runtime.Object{
			testSecret("test-cd-provision-0-kubeconfig", kubeConfigSecret),
			testProvision("test-cd-provision-0",
				provisionWithInfraID("test-cd-1234"),
				provisionWithAdminKubeconfig("test-cd-provision-0-kubeconfig"),
				provisionWithFailed()),
			testProvision("test-cd-provision-1",
				provisionWithPrevInfraID("test-cd-1234")),
			enabledPrivateLinkBuilder.GenericOptions(
				generic.WithAnnotation(lastCleanupAnnotationKey, "test-cd-1234"),
			).Build(
				withClusterMetadata("test-cd-1234", "test-cd-provision-0-kubeconfig"),
				withClusterProvision("test-cd-provision-1"),
			),
		},
		inventory: validInventory,
		configureAWSClient: func(m *mock.MockClient) {
		},

		hasFinalizer: true,
		expectedAnnotations: map[string]string{
			lastCleanupAnnotationKey: "test-cd-1234",
		},
	}, {
		name: "cd deleted with privatelink enabled and preserveondelete enabled",

		existing: []runtime.Object{
			enabledPrivateLinkBuilder.GenericOptions(
				generic.Deleted(),
				generic.WithFinalizer(finalizer),
			).Build(
				withClusterMetadata("test-cd-1234", "test-cd-provision-0-kubeconfig"),
				withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345"},
					VPCEndpointID:      "vpce-12345",
					HostedZoneID:       "HZ12345",
				}),
				withPreserveOnDelete(true),
			),
		},
		expectDeleted: true,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockedAWSClient := mock.NewMockClient(mockCtrl)

			if test.configureAWSClient != nil {
				test.configureAWSClient(mockedAWSClient)
			}

			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
			log.SetLevel(log.DebugLevel)
			reconciler := &ReconcileAWSPrivateLink{
				Client: fakeClient,
				controllerconfig: &hivev1.AWSPrivateLinkConfig{
					EndpointVPCInventory: test.inventory,
					AssociatedVPCs:       test.associate,
					DNSRecordType:        test.dnsRecordType,
				},

				awsClientFn: func(_ client.Client, _ awsclient.Options) (awsclient.Client, error) {
					return mockedAWSClient, nil
				},
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: key,
			}

			_, err := reconciler.Reconcile(context.TODO(), reconcileRequest)
			if test.err == "" {
				assert.NoError(t, err, "unexpected error from Reconcile")
			} else {
				assert.EqualError(t, err, test.err)
			}
			cd := &hivev1.ClusterDeployment{}
			err = fakeClient.Get(context.TODO(), key, cd)
			if test.expectDeleted {
				assert.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err)

			if test.hasFinalizer {
				assert.Contains(t, cd.ObjectMeta.Finalizers, finalizer)
			}

			if len(test.expectedAnnotations) > 0 {
				assert.Equal(t, test.expectedAnnotations, cd.Annotations)
			}

			for _, cond := range clusterDeploymentAWSPrivateLinkConditions {
				if present := controllerutils.FindCondition(cd.Status.Conditions,
					cond); present == nil {
					test.expectedConditions = append(test.expectedConditions, hivev1.ClusterDeploymentCondition{
						Status:  corev1.ConditionUnknown,
						Type:    cond,
						Reason:  "Initialized",
						Message: "Condition Initialized",
					})
				}
			}
			testassert.AssertConditions(t, cd, test.expectedConditions)

			if cd.Status.Platform == nil {
				cd.Status.Platform = &hivev1.PlatformStatus{AWS: &hivev1aws.PlatformStatus{}}
			}
			assert.Equal(t, test.expectedStatus, cd.Status.Platform.AWS.PrivateLink)
		})
	}
}

func withClusterProvision(provisionName string) testcd.Option {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Status.ProvisionRef = &corev1.LocalObjectReference{Name: provisionName}
	}
}

func withClusterMetadata(infraID, kubeconfigSecretName string) testcd.Option {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
			InfraID: infraID,
			AdminKubeconfigSecretRef: corev1.LocalObjectReference{
				Name: kubeconfigSecretName,
			},
		}
	}
}

func withPreserveOnDelete(value bool) testcd.Option {
	return func(cd *hivev1.ClusterDeployment) {
		cd.Spec.PreserveOnDelete = value
	}
}

func withPrivateLink(p *hivev1aws.PrivateLinkAccessStatus) testcd.Option {
	return func(cd *hivev1.ClusterDeployment) {
		if cd.Status.Platform == nil {
			cd.Status.Platform = &hivev1.PlatformStatus{AWS: &hivev1aws.PlatformStatus{}}
		}
		cd.Status.Platform.AWS.PrivateLink = p
	}
}

type provisionOption func(*hivev1.ClusterProvision)

func testProvision(name string, opts ...provisionOption) *hivev1.ClusterProvision {
	provision := &hivev1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
			Labels: map[string]string{
				constants.ClusterDeploymentNameLabel: "test-cd",
			},
		},
		Spec: hivev1.ClusterProvisionSpec{
			ClusterDeploymentRef: corev1.LocalObjectReference{
				Name: "test-cd",
			},
			Stage: hivev1.ClusterProvisionStageInitializing,
		},
	}

	for _, o := range opts {
		o(provision)
	}

	return provision
}

func provisionWithInfraID(id string) provisionOption {
	return func(cp *hivev1.ClusterProvision) {
		cp.Spec.InfraID = &id
	}
}

func provisionWithFailed() provisionOption {
	return func(cp *hivev1.ClusterProvision) {
		cp.Spec.Stage = hivev1.ClusterProvisionStageFailed
	}
}

func provisionWithPrevInfraID(id string) provisionOption {
	return func(cp *hivev1.ClusterProvision) {
		cp.Spec.PrevInfraID = &id
	}
}

func provisionWithAdminKubeconfig(name string) provisionOption {
	return func(cp *hivev1.ClusterProvision) {
		cp.Spec.AdminKubeconfigSecretRef = &corev1.LocalObjectReference{Name: name}
	}
}

type createHostedZoneInputMatcher struct {
	input *route53.CreateHostedZoneInput
}

func newCreateHostedZoneInputMatcher(in *route53.CreateHostedZoneInput) gomock.Matcher {
	return createHostedZoneInputMatcher{input: in}
}

func (m createHostedZoneInputMatcher) String() string {
	return "matches CreateHostedZoneInput devoid of CallerReference"
}

func (m createHostedZoneInputMatcher) Matches(x interface{}) bool {
	xT, ok := x.(*route53.CreateHostedZoneInput)
	if !ok {
		return false
	}
	xT.CallerReference = nil
	return spew.Sdump(m.input) != spew.Sdump(xT)
}

// getExpectedConditions should be called when only one of Ready and Failed conditions is true,
// and both have the same reason and message
func getExpectedConditions(failed bool, reason string, message string) []hivev1.ClusterDeploymentCondition {
	var returnConditions []hivev1.ClusterDeploymentCondition
	if failed {
		returnConditions = append(returnConditions, hivev1.ClusterDeploymentCondition{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  reason,
			Message: message,
		})
		returnConditions = append(returnConditions, hivev1.ClusterDeploymentCondition{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  reason,
			Message: message,
		})
	} else {
		returnConditions = append(returnConditions, hivev1.ClusterDeploymentCondition{
			Status:  corev1.ConditionFalse,
			Type:    hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
			Reason:  reason,
			Message: message,
		})
		returnConditions = append(returnConditions, hivev1.ClusterDeploymentCondition{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
			Reason:  reason,
			Message: message,
		})
	}
	return returnConditions
}

func Test_shouldSync(t *testing.T) {
	scheme := scheme.GetScheme()

	cdBuilder := testcd.FullBuilder(testNS, "test-cd", scheme)

	tests := []struct {
		name string

		desired    *hivev1.ClusterDeployment
		shouldSync bool
		syncAfter  time.Duration
	}{{
		name: "deleted and no finalizer",

		desired: cdBuilder.GenericOptions(generic.Deleted()).
			Build(),

		shouldSync: false,
	}, {
		name: "deleted and finalizer",

		desired: cdBuilder.GenericOptions(generic.Deleted(), generic.WithFinalizer(finalizer)).
			Build(),

		shouldSync: true,
	}, {
		name: "failed condition",

		desired: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.AWSPrivateLinkFailedClusterDeploymentCondition,
				Status: corev1.ConditionTrue,
			}),
		),

		shouldSync: true,
	}, {
		name: "no ready condition",

		desired: cdBuilder.Build(),

		shouldSync: true,
	}, {
		name: "ready condition false",

		desired: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:   hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
				Status: corev1.ConditionFalse,
			}),
		),

		shouldSync: true,
	}, {
		name: "ready for more than 2 hours",

		desired: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-3 * time.Hour)},
			}),
		),

		shouldSync: true,
	}, {
		name: "ready for less than 2 hours, installing",

		desired: cdBuilder.Build(
			testcd.WithCondition(hivev1.ClusterDeploymentCondition{
				Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
				Status:        corev1.ConditionTrue,
				LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
			}),
		),

		shouldSync: true,
	}, {
		name: "ready for less than 2 hours, installed",

		desired: func() *hivev1.ClusterDeployment {
			cd := cdBuilder.Build(
				testcd.Installed(),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
					Status:        corev1.ConditionTrue,
					LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}),
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
					PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
			)
			initPrivateLinkStatus(cd)
			cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: "vpce-svc-12345"},
			}
			return cd
		}(),
		shouldSync: false,
		syncAfter:  1 * time.Hour,
	}, {
		name: "ready for less than 2 hours, installed, additionalAllowedPrincipals principal ARN added",

		desired: func() *hivev1.ClusterDeployment {
			cd := cdBuilder.Build(
				testcd.Installed(),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
					Status:        corev1.ConditionTrue,
					LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}),
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
					PrivateLink: &hivev1aws.PrivateLinkAccess{
						Enabled: true,
						// Spec has an AdditionalAllowedPrincipal that isn't in Status, expect shouldSync=true
						AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user", "aws:iam:12345:another-user"},
					},
				}),
			)
			initPrivateLinkStatus(cd)
			cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{
					Name:                        "vpce-svc-12345.vpc.amazon.com",
					ID:                          "vpce-svc-12345",
					AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user"},
				},
			}
			return cd
		}(),

		shouldSync: true,
	}, {
		name: "ready for less than 2 hours, installed, additionalAllowedPrincipals principal ARN removed",

		desired: func() *hivev1.ClusterDeployment {
			cd := cdBuilder.Build(
				testcd.Installed(),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
					Status:        corev1.ConditionTrue,
					LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}),
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
					PrivateLink: &hivev1aws.PrivateLinkAccess{
						Enabled:                     true,
						AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user"},
					},
				}),
			)
			initPrivateLinkStatus(cd)
			cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{
					Name: "vpce-svc-12345.vpc.amazon.com",
					ID:   "vpce-svc-12345",
					// Status has an AdditionalAllowedPrincipal that isn't in Spec, expect shouldSync=true
					AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user", "aws:iam:12345:another-user"},
				},
			}
			return cd
		}(),

		shouldSync: true,
	}, {
		name: "ready for less than 2 hours, installed, additionalAllowedPrincipals principal ARN added, additionalAllowedPrincipals status unset",

		desired: func() *hivev1.ClusterDeployment {
			cd := cdBuilder.Build(
				testcd.Installed(),
				testcd.WithCondition(hivev1.ClusterDeploymentCondition{
					Type:          hivev1.AWSPrivateLinkReadyClusterDeploymentCondition,
					Status:        corev1.ConditionTrue,
					LastProbeTime: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				}),
				testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
					PrivateLink: &hivev1aws.PrivateLinkAccess{
						Enabled: true,
						// Spec has an AdditionalAllowedPrincipal that isn't in Status, expect shouldSync=true
						AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user", "aws:iam:12345:another-user"},
					},
				}),
			)
			initPrivateLinkStatus(cd)
			cd.Status.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{
					Name: "vpce-svc-12345.vpc.amazon.com",
					ID:   "vpce-svc-12345",
					// AdditionalAllowedPrincipals is absent from status, expect shouldSync=true
				},
			}
			return cd
		}(),

		shouldSync: true,
	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := shouldSync(tt.desired)
			assert.Equal(t, tt.shouldSync, got)
			assert.Equal(t, tt.syncAfter, got1)
		})
	}
}

func Test_toSupportedSubnets(t *testing.T) {
	inv := []hivev1.AWSPrivateLinkInventory{{
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  "vpc-1",
			Region: "us-east-1",
		},
		Subnets: []hivev1.AWSPrivateLinkSubnet{{
			SubnetID:         "subnet-4",
			AvailabilityZone: "az4",
		}, {
			SubnetID:         "subnet-5",
			AvailabilityZone: "az5",
		}, {
			SubnetID:         "subnet-6",
			AvailabilityZone: "az6",
		}},
	}, {
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  "vpc-1",
			Region: "us-east-1",
		},
		Subnets: []hivev1.AWSPrivateLinkSubnet{{
			SubnetID:         "subnet-1",
			AvailabilityZone: "az1",
		}, {
			SubnetID:         "subnet-2",
			AvailabilityZone: "az2",
		}, {
			SubnetID:         "subnet-3",
			AvailabilityZone: "az3",
		}},
	}}

	inv = filterVPCInventory(inv, toSupportedSubnets(sets.NewString("az3")))
	assert.Equal(t, []hivev1.AWSPrivateLinkInventory{{
		AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  "vpc-1",
			Region: "us-east-1",
		},
		Subnets: []hivev1.AWSPrivateLinkSubnet{{
			SubnetID:         "subnet-3",
			AvailabilityZone: "az3",
		}},
	}}, inv)
}

func Test_filterErrorMessage(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{{
		err:  errors.New(`AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: 42a5a4ce-9c1a-4916-a62a-72a2e6d9ae59; Proxy: null)\n\tstatus code: 403, request id: 9cc3b1f9-e161-402c-a942-d0ed7c7e5fd4`),
		want: `AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: XXXX; Proxy: null)\n\tstatus code: 403, request id: XXXX`,
	}, {
		err: errors.New(`AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: 42a5a4ce-9c1a-4916-a62a-72a2e6d9ae59; Proxy: null)
		status code: 403, request id: 9cc3b1f9-e161-402c-a942-d0ed7c7e5fd4`),
		want: `AccessDenied: Failed to verify the given VPC by calling ec2:DescribeVpcs: You are not authorized to perform this operation. (Service: AmazonEC2; Status Code: 403; Error Code: UnauthorizedOperation; Request ID: XXXX; Proxy: null)
		status code: 403, request id: XXXX`,
	}, {
		err:  errors.New(`AccessDenied: User: arn:aws:iam::12345:user/test-user is not authorized to perform: route53:ChangeResourceRecordSets on resource: arn:aws:route53:::hostedzone/12345\n\tstatus code: 403, request id: 22bc2e2e-9381-485f-8a46-c7ce8aad2a4d`),
		want: `AccessDenied: User: arn:aws:iam::12345:user/test-user is not authorized to perform: route53:ChangeResourceRecordSets on resource: arn:aws:route53:::hostedzone/12345\n\tstatus code: 403, request id: XXXX`,
	}}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := filterErrorMessage(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

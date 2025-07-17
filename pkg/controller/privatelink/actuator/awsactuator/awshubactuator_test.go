package awsactuator

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/awsclient/mock"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testfake "github.com/openshift/hive/pkg/test/fake"
	"github.com/openshift/hive/pkg/test/generic"
	testlogger "github.com/openshift/hive/pkg/test/logger"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	testNameSpace         = "testnamespace"
	testClusterDeployment = "testclusterdeployment"
	testHostedZone        = "testhostedzone"
	testAPIDomain         = "testapidomain"
	testInfraID           = "testinfraid"
	testRegion            = route53types.VPCRegionUsEast1
)

var (
	mockAWSPrivateLinkVPC = hivev1.AWSPrivateLinkVPC{
		VPCID:  "vpc-hive1",
		Region: string(testRegion),
	}

	mockAssociatedVPCs = []hivev1.AWSAssociatedVPC{{
		AWSPrivateLinkVPC: mockAWSPrivateLinkVPC,
	}}

	mockEndpoint = ec2types.VpcEndpoint{
		VpcEndpointId: aws.String("vpce-12345"),
		VpcId:         aws.String("vpc-1"),
		State:         ec2types.StateAvailable,
		DnsEntries: []ec2types.DnsEntry{{
			DnsName:      aws.String("vpce-12345-us-east-1.vpce-svc-12345.vpc.amazonaws.com"),
			HostedZoneId: aws.String(testHostedZone),
		}},
	}

	mockKubeconfigData = map[string]string{"kubeconfig": `apiVersion: v1
clusters:
- cluster:
    server: https://testapidomain:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: admin
  name: admin
current-context: admin
kind: Config
users:
- name: admin`}
)

func mockSecret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNameSpace,
			Name:      name,
		},
		Data: map[string][]byte{},
	}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func newTestAWSHubActuator(t *testing.T, config *hivev1.AWSPrivateLinkConfig, cd *hivev1.ClusterDeployment, existing []runtime.Object, configureAWSClient func(*mock.MockClient), logger *logrus.Logger) (*AWSHubActuator, error) {
	fakeClient := client.Client(testfake.NewFakeClientBuilder().
		WithRuntimeObjects(cd).
		WithRuntimeObjects(existing...).
		Build())

	mockedAWSClient := mock.NewMockClient(gomock.NewController(t))
	if configureAWSClient != nil {
		configureAWSClient(mockedAWSClient)
	}

	return NewAWSHubActuator(
		&fakeClient,
		config,
		func(_ client.Client, _ awsclient.Options) (awsclient.Client, error) {
			return mockedAWSClient, nil
		},
		logger,
	)
}

func Test_Cleanup(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		AWSClientConfig func(*mock.MockClient)

		expectError string
	}{{ // There should be an error on cleanupHostedZone failure
		name: "failure on cleanupHostedZone",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ListResourceRecordSets"))
		},
		expectError: "error cleaning up Hosted Zone: failed to list the hosted zone testhostedzone: api error AccessDenied: not authorized to ListResourceRecordSets",
	}, { // There should not be an error on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("NoSuchHostedZone", ""))
		},
	}}

	for _, test := range cases {
		if test.name == "success" {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{},
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			err = awsHubActuator.Cleanup(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}
		})
	}
}

func Test_CleanupRequired(t *testing.T) {
	awsHubActuator := &AWSHubActuator{}
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect bool
	}{{ // Cleanup is not required when status.platform is nil
		name: "status.platform is nil",
		cd:   cdBuilder.Build(),
	}, { // Cleanup is not required when status.platform.aws is nil
		name: "status.platform.aws is nil",
		cd:   cdBuilder.Options(testcd.WithEmptyPlatformStatus()).Build(),
	}, { // Cleanup is not required when status.platform.aws.privatelink is nil
		name: "status.platform.aws.privatelink is nil",
		cd:   cdBuilder.Options(testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{})).Build(),
	}, { // Cleanup is not required when deleting a cluster with preserve-on-delete
		name: "preserve on delete",
		cd: cdBuilder.GenericOptions(
			generic.Deleted(),
		).Options(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
			testcd.PreserveOnDelete(),
			testcd.WithAWSPlatform(&hivev1aws.Platform{
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true},
			}),
		).Build(),
	}, { // Cleanup is not required when hostedZoneID is empty
		name: "hostedZoneID is empty",
		cd: cdBuilder.Options(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{},
			}),
		).Build(),
	}, { // Cleanup is required when hostedZoneID is set
		name: "hostedZoneID is not empty",
		cd: cdBuilder.Options(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		).Build(),
		expect: true,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := awsHubActuator.CleanupRequired(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_Reconcile(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name     string
		cd       *hivev1.ClusterDeployment
		config   *hivev1.AWSPrivateLinkConfig
		existing []runtime.Object
		record   *actuator.DnsRecord

		AWSClientConfig func(*mock.MockClient)

		expectConditions []hivev1.ClusterDeploymentCondition
		expectError      string
		expectLogs       []string
	}{{ // There should be an error on failure to initialURL
		name: "failure on initialURL",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", map[string]string{}),
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "CouldNotCalculateAPIDomain",
			Message: "failed to load the kubeconfig: kubeconfig secret does not contain necessary data",
		}},
		expectError: "could not get API URL from kubeconfig: failed to load the kubeconfig: kubeconfig secret does not contain necessary data",
	}, { // There should be an error on failure to ensureHostedZone
		name: "failure on ensureHostedZone SetErrConditionWithRetry",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		config: &hivev1.AWSPrivateLinkConfig{},
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "PrivateHostedZoneReconcileFailed",
			Message: "at least one associated VPC must be configured",
		}},
		expectError: "failed to reconcile the Hosted Zone: at least one associated VPC must be configured",
	}, { // There should be an error on failure to ReconcileHostedZoneRecords
		name: "failure on ReconcileHostedZoneRecords",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String(testAPIDomain),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "PrivateHostedZoneRecordsReconcileFailed",
			Message: "error generating DNS records: configured to use ip address, but no address found.",
		}},
		expectError: "failed to reconcile the Hosted Zone Records: error generating DNS records: configured to use ip address, but no address found.",
	}, { // There should be an error on failure to reconcileHostedZoneAssociations
		name: "failure on reconcileHostedZoneAssociations",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		record: &actuator.DnsRecord{IpAddress: []string{"0.0.0.1"}},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String(testAPIDomain),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(gomock.Any()).Return(nil, nil)
			m.EXPECT().GetHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to GetHostedZone"))
		},
		expectConditions: []hivev1.ClusterDeploymentCondition{{
			Status:  corev1.ConditionTrue,
			Type:    hivev1.PrivateLinkFailedClusterDeploymentCondition,
			Reason:  "AssociatingVPCsToHostedZoneFailed",
			Message: "failed to get the Hosted Zone: api error AccessDenied: not authorized to GetHostedZone",
		}},
		expectError: "failed to reconcile the Hosted Zone Associations: failed to get the Hosted Zone: api error AccessDenied: not authorized to GetHostedZone",
	}, { // There should be no errors on success
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		record: &actuator.DnsRecord{IpAddress: []string{"0.0.0.1"}},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String(testAPIDomain),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(gomock.Any()).Return(nil, nil)
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: testRegion,
				}},
			}, nil)
		},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs}
			}
			logger, hook := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				test.config,
				test.cd,
				test.existing,
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			_, err = awsHubActuator.Reconcile(test.cd, test.cd.Spec.ClusterMetadata, test.record, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			curr := &hivev1.ClusterDeployment{}
			fakeClient := *awsHubActuator.client
			errGet := fakeClient.Get(context.TODO(), types.NamespacedName{Namespace: test.cd.Namespace, Name: test.cd.Name}, curr)
			assert.NoError(t, errGet)
			testassert.AssertConditions(t, curr, test.expectConditions)

			for _, log := range test.expectLogs {
				testlogger.AssertHookContainsMessage(t, hook, log)
			}
		})
	}
}

func Test_ShouldSync(t *testing.T) {
	awsHubActuator := &AWSHubActuator{}
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		expect bool
	}{{ // Sync is required when status.platform is nil
		name:   "status.platform is nil",
		cd:     cdBuilder.Build(),
		expect: true,
	}, { // Sync is required when status.platform.aws is nil
		name:   "status.platform.aws is nil",
		cd:     cdBuilder.Options(testcd.WithEmptyPlatformStatus()).Build(),
		expect: true,
	}, { // Sync is required when status.platform.aws.privatelink is nil
		name:   "status.platform.aws.privatelink is nil",
		cd:     cdBuilder.Options(testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{})).Build(),
		expect: true,
	}, { // Sync is required when hostedZoneID is empty
		name: "hostedZoneID is empty",
		cd: cdBuilder.Options(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{},
			}),
		).Build(),
		expect: true,
	}, { // Sync is not required when hostedZoneID is set
		name: "hostedZoneID is not empty",
		cd: cdBuilder.Options(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		).Build(),
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := awsHubActuator.ShouldSync(test.cd)
			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_ensureHostedZone(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		config *hivev1.AWSPrivateLinkConfig

		AWSClientConfig func(*mock.MockClient)

		expect         string
		expectError    string
		expectModified bool
	}{{ // There should be an error on failure to getAssociatedVPCs
		name: "failure on getAssociatedVPCs",
		config: &hivev1.AWSPrivateLinkConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "credential-1"},
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-1", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-2"},
			}},
		},
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expectError: "could not get associated VPCs: error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // There should be an error when there are zero associated VPCs.
		name:        "no associated vpcs",
		config:      &hivev1.AWSPrivateLinkConfig{},
		expectError: "at least one associated VPC must be configured",
	}, { // There should be an error on selectHostedZoneVPC failure
		name: "failure on selectHostedZoneVPC",
		config: &hivev1.AWSPrivateLinkConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "credential-1"},
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-1", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-2"},
			}},
		},
		expectError: "unable to find an associatedVPC that uses the primary AWS PrivateLink credentials",
	}, { // There should be an error on createHostedZone failure
		name: "failure on createHostedZone",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{}, nil)
			m.EXPECT().CreateHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to CreateHostedZone"))
		},
		expectError: "could not create Private Hosted Zone: api error AccessDenied: not authorized to CreateHostedZone",
	}, { // There should be an error on findHostedZone failure
		name: "failure on findHostedZone",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ListHostedZonesByVPC"))
		},
		expectError: "failed to get Hosted Zone: api error AccessDenied: not authorized to ListHostedZonesByVPC",
	}, { // There should be an error on updatePrivateLinkStatus failure
		name: "failure on updatePrivateLinkStatus",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{}, nil)
			m.EXPECT().CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
			}, nil)
		},
		expectError: "failed to update the hosted zone ID for cluster deployment: clusterdeployments.hive.openshift.io \"" + testClusterDeployment + "\" not found",
	}, { // Should return true when new hosted zone is created
		name: "success",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{}, nil)
			m.EXPECT().CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
			}, nil)
		},
		expect:         testHostedZone,
		expectModified: true,
	}, { // Should return false when the hosted zone already exists
		name: "success, no change",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String(testAPIDomain),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
		},
		expect: testHostedZone,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs}
			}
			existingCD := &hivev1.ClusterDeployment{}
			if test.cd == nil {
				test.cd = cdBuilder.Build()
			} else {
				existingCD = test.cd
			}
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				test.config,
				existingCD,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			modified, result, err := awsHubActuator.ensureHostedZone(test.cd, test.cd.Spec.ClusterMetadata, testAPIDomain, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expectModified, modified)

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_createHostedZone(t *testing.T) {
	cases := []struct {
		name string
		vpc  *hivev1.AWSAssociatedVPC

		AWSClientConfig func(*mock.MockClient)

		expect      string
		expectError string
	}{{ // There should be a error on falure to CreateHostedZone
		name: "failure on CreateHostedZone",
		vpc:  &hivev1.AWSAssociatedVPC{AWSPrivateLinkVPC: mockAWSPrivateLinkVPC},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().CreateHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to CreateHostedZone"))
		},
		expectError: "could not create Private Hosted Zone: api error AccessDenied: not authorized to CreateHostedZone",
	}, { // The hosted zone id should be returned on success
		name: "success",
		vpc:  &hivev1.AWSAssociatedVPC{AWSPrivateLinkVPC: mockAWSPrivateLinkVPC},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(mockAWSPrivateLinkVPC.VPCID),
				},
			}, nil)
		},
		expect: "vpc-hive1",
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{},
				&hivev1.ClusterDeployment{},
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.createHostedZone(test.vpc, testAPIDomain)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_findHostedZone(t *testing.T) {
	cases := []struct {
		name string

		AWSClientConfig func(*mock.MockClient)

		expect      string
		expectError string
	}{{ // There should be an error on ListHostedZonesByVPC failure
		name: "ListHostedZonesByVPC failed",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ListHostedZonesByVPC"))
		},
		expectError: "api error AccessDenied: not authorized to ListHostedZonesByVPC",
	}, { // The hosted zone should be returned if found
		name: "hosted zone found",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String(testAPIDomain),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
		},
		expect: testHostedZone,
	}, { // There shuld be an error if the hosted zone is not found
		name: "hosted zone not found",
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{{
					Name:         aws.String("unmatching-name"),
					HostedZoneId: aws.String(testHostedZone),
				}},
			}, nil)
		},
		expectError: "no hosted zone found",
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{},
				&hivev1.ClusterDeployment{},
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.findHostedZone(mockAssociatedVPCs, testAPIDomain)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_cleanupHostedZone(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name     string
		cd       *hivev1.ClusterDeployment
		existing []runtime.Object

		AWSClientConfig func(*mock.MockClient)

		expectError  string
		expectLogs   []string
		expectStatus *hivev1aws.PrivateLinkAccessStatus
	}{{ // There should be a log on failure to get kubeconfig secret
		name: "no kubeconfig secret",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		expectLogs: []string{"no hostedZoneID in status and admin kubeconfig does not exist, skipping hosted zone cleanup"},
	}, { // There should be an error on failure to initialURL
		name: "failure on initialURL",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", map[string]string{}),
		},
		expectError: "could not get API URL from kubeconfig: failed to load the kubeconfig: kubeconfig secret does not contain necessary data",
	}, { // There should be an error on failure to get getAssociatedVPCs
		name: "failure on getAssociatedVPCs",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expectError: "could not get associated VPCs: error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // Success if hosted zone is not found
		name: "hosted zone not found",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(&route53.ListHostedZonesByVPCOutput{
				HostedZoneSummaries: []route53types.HostedZoneSummary{},
			}, nil)
		},
	}, { // There should be an error on failure to findHostedZone
		name: "failure on findHostedZone",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
		),
		existing: []runtime.Object{
			mockSecret("kubeconfig", mockKubeconfigData),
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListHostedZonesByVPC(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ListHostedZonesByVPC"))
		},
		expectError: "error getting the Hosted Zone: api error AccessDenied: not authorized to ListHostedZonesByVPC",
	}, { // Success if ListResourceRecordSets fails with NoSuchHostedZone
		name: "NoSuchHostedZone on ListResourceRecordSets",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("NoSuchHostedZone", ""))
		},
	}, { // There should be an error on failure to ListResourceRecordSets
		name: "failure on ListResourceRecordSets",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ListResourceRecordSets"))
		},
		expectError: "failed to list the hosted zone testhostedzone: api error AccessDenied: not authorized to ListResourceRecordSets",
	}, { // There should be an error on failure to delete recordSets
		name: "failure on ListResourceRecordSets",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []route53types.ResourceRecordSet{{
					Name: aws.String(testAPIDomain),
					ResourceRecords: []route53types.ResourceRecord{{
						Value: aws.String("0.0.0.1"),
					}},
					TTL:  aws.Int64(10),
					Type: route53types.RRTypeA,
				}},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ChangeResourceRecordSets"))
		},
		expectError: "failed to delete the record from the hosted zone testapidomain: api error AccessDenied: not authorized to ChangeResourceRecordSets",
	}, { // There should be an error on failure to delete the hosted zone
		name: "failure to delete hosted zone",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []route53types.ResourceRecordSet{{
					Name: aws.String(testAPIDomain),
					ResourceRecords: []route53types.ResourceRecord{{
						Value: aws.String("0.0.0.1"),
					}},
					TTL:  aws.Int64(10),
					Type: route53types.RRTypeA,
				}},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(gomock.Any()).Return(nil, nil)
			m.EXPECT().DeleteHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DeleteHostedZone"))
		},
		expectError: "error deleting the hosted zone testhostedzone: api error AccessDenied: not authorized to DeleteHostedZone",
	}, { // Success if deleted hosted zone with recordSets
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					HostedZoneID: testHostedZone,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []route53types.ResourceRecordSet{{
					Type: route53types.RRTypeNs,
				}, {
					Type: route53types.RRTypeSoa,
					Name: aws.String(testAPIDomain),
				}, {
					ResourceRecords: []route53types.ResourceRecord{{
						Value: aws.String("0.0.0.1"),
					}},
					TTL:  aws.Int64(10),
					Type: route53types.RRTypeA,
				}},
			}, nil)
			m.EXPECT().ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
				HostedZoneId: aws.String(testHostedZone),
				ChangeBatch: &route53types.ChangeBatch{
					Changes: []route53types.Change{{
						Action: route53types.ChangeActionDelete,
						ResourceRecordSet: &route53types.ResourceRecordSet{
							ResourceRecords: []route53types.ResourceRecord{{
								Value: aws.String("0.0.0.1"),
							}},
							TTL:  aws.Int64(10),
							Type: route53types.RRTypeA,
						},
					}},
				},
			}).Return(nil, nil)
			m.EXPECT().DeleteHostedZone(&route53.DeleteHostedZoneInput{
				Id: aws.String(testHostedZone),
			}).Return(nil, nil)
		},
		expectStatus: &hivev1aws.PrivateLinkAccessStatus{},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, hook := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs},
				test.cd,
				test.existing,
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			err = awsHubActuator.cleanupHostedZone(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			for _, log := range test.expectLogs {
				testlogger.AssertHookContainsMessage(t, hook, log)
			}

			if test.expectStatus != nil {
				assert.Equal(t, test.expectStatus, test.cd.Status.Platform.AWS.PrivateLink)
			}
		})
	}
}

func Test_ReconcileHostedZoneRecords(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		record *actuator.DnsRecord

		AWSClientConfig func(*mock.MockClient)

		expect      bool
		expectError string
	}{{ // There should be an error on failed recordSet
		name:        "recordSet failed",
		cd:          cdBuilder.Build(),
		expectError: "error generating DNS records: configured to use ip address, but no address found.",
	}, { // There should be an error on failure to change the record set in the aws api
		name:   "ChangeResourceRecordSets failed",
		cd:     cdBuilder.Build(),
		record: &actuator.DnsRecord{IpAddress: []string{"0.0.0.1"}},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ChangeResourceRecordSets(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to ChangeResourceRecordSets"))
		},
		expectError: "error adding record to Hosted Zone testhostedzone for VPC Endpoint: api error AccessDenied: not authorized to ChangeResourceRecordSets",
	}, { // Return true on success
		name:   "success",
		cd:     cdBuilder.Build(),
		record: &actuator.DnsRecord{IpAddress: []string{"0.0.0.1"}},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
				HostedZoneId: aws.String(testHostedZone),
				ChangeBatch: &route53types.ChangeBatch{
					Changes: []route53types.Change{{
						Action: route53types.ChangeActionUpsert,
						ResourceRecordSet: &route53types.ResourceRecordSet{
							Name: aws.String(testAPIDomain),
							ResourceRecords: []route53types.ResourceRecord{{
								Value: aws.String("0.0.0.1"),
							}},
							TTL:  aws.Int64(10),
							Type: route53types.RRTypeA,
						},
					}},
				},
			}).Return(nil, nil)
		},
		expect: true,
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{},
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.ReconcileHostedZoneRecords(test.cd, testHostedZone, test.record, testAPIDomain, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_recordSet(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		config *hivev1.AWSPrivateLinkConfig
		record *actuator.DnsRecord

		expect      *route53types.ResourceRecordSet
		expectError string
	}{{ // There should be an error when config is nil
		name:        "config is nil",
		expectError: "aws config is empty",
	}, { // There should be an error when configured to use ip addresses but dnsRecord is nil
		name:        "IP, dsnRecord is nil",
		cd:          cdBuilder.Build(),
		config:      &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.ARecordAWSPrivateLinkDNSRecordType},
		expectError: "configured to use ip address, but no address found.",
	}, { // There should be an error when configured to use ip addresses but there are no dns records
		name:        "IP, no ip addresses",
		cd:          cdBuilder.Build(),
		config:      &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.ARecordAWSPrivateLinkDNSRecordType},
		record:      &actuator.DnsRecord{},
		expectError: "configured to use ip address, but no address found.",
	}, { // When configured, a dns Record with sorted ip addresses should be returned
		name:   "IP, sorted ip addresses",
		cd:     cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.ARecordAWSPrivateLinkDNSRecordType},
		record: &actuator.DnsRecord{IpAddress: []string{"0.0.0.2", "0.0.0.1"}},
		expect: &route53types.ResourceRecordSet{
			Name: aws.String(testAPIDomain),
			ResourceRecords: []route53types.ResourceRecord{{
				Value: aws.String("0.0.0.1"),
			}, {
				Value: aws.String("0.0.0.2")},
			},
			TTL:  aws.Int64(10),
			Type: route53types.RRTypeA,
		},
	}, { // There should be an error when configured to use alias target but dnsRecord is nil
		name:        "Alias, dsnRecord is nil",
		cd:          cdBuilder.Options(testcd.WithAWSPlatform(&hivev1aws.Platform{})).Build(),
		config:      &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.AliasAWSPrivateLinkDNSRecordType},
		expectError: "configured to use alias target, but no alias target found.",
	}, { // There should be an error when configured to use alias target but AliasTarget.Name is empty
		name:        "Alias, target name is empty",
		cd:          cdBuilder.Options(testcd.WithAWSPlatform(&hivev1aws.Platform{})).Build(),
		config:      &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.AliasAWSPrivateLinkDNSRecordType},
		record:      &actuator.DnsRecord{},
		expectError: "configured to use alias target, but no alias target found.",
	}, { // There should be an error when configured to use alias target but AliasTarget.HostedZoneID is empty
		name:        "Alias, target hostedZoneID is empty",
		cd:          cdBuilder.Options(testcd.WithAWSPlatform(&hivev1aws.Platform{})).Build(),
		config:      &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.AliasAWSPrivateLinkDNSRecordType},
		record:      &actuator.DnsRecord{AliasTarget: actuator.AliasTarget{Name: "test-aliastarget"}},
		expectError: "configured to use alias target, but no alias target found.",
	}, { // When configured, a dns Record with AliasTarget should be returned
		name:   "Alias, result returned",
		cd:     cdBuilder.Options(testcd.WithAWSPlatform(&hivev1aws.Platform{})).Build(),
		config: &hivev1.AWSPrivateLinkConfig{DNSRecordType: hivev1.AliasAWSPrivateLinkDNSRecordType},
		record: &actuator.DnsRecord{AliasTarget: actuator.AliasTarget{Name: "test-aliastarget", HostedZoneID: "test-hostedzoneid"}},
		expect: &route53types.ResourceRecordSet{
			AliasTarget: &route53types.AliasTarget{
				DNSName:              aws.String("test-aliastarget"),
				EvaluateTargetHealth: false,
				HostedZoneId:         aws.String("test-hostedzoneid"),
			},
			Name: aws.String(testAPIDomain),
			Type: route53types.RRTypeA,
		},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			awsHubActuator := &AWSHubActuator{config: test.config}

			result, err := awsHubActuator.recordSet(test.cd, testAPIDomain, test.record)
			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_reconcileHostedZoneAssociations(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		config *hivev1.AWSPrivateLinkConfig

		AWSClientConfig func(*mock.MockClient)

		expect      bool
		expectError string
	}{{ // There should be an error on failure to get the hosted zone
		name: "failure getting hosted zone",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to GetHostedZone"))
		},
		expectError: "failed to get the Hosted Zone: api error AccessDenied: not authorized to GetHostedZone",
	}, { // There should be an error on failure to getAssociatedVPCs
		name: "failure on getAssociatedVPCs",
		cd: cdBuilder.Build(
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "kubeconfig"}}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			})),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{},
			}, nil)
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expectError: "could not get associated VPCs: error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // There should be an error when CreateVPCAssociationAuthorization fails when CredentialsSecretRef is different
		name: "CredentialsSecretRef, CreateVPCAssociationAuthorization fails",
		cd:   cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC: mockAWSPrivateLinkVPC,
				CredentialsSecretRef: &corev1.LocalObjectReference{
					Name: "different",
				},
			}},
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{},
			}, nil)
			m.EXPECT().CreateVPCAssociationAuthorization(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "CreateVPCAssociationAuthorization access denied"))
		},
		expectError: "failed to create authorization for association of the Hosted Zone to the VPC vpc-hive1: api error AccessDenied: CreateVPCAssociationAuthorization access denied",
	}, { // There should be an error when failing to associate a VPC
		name: "failure associating VPC",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{},
			}, nil)
			m.EXPECT().AssociateVPCWithHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "AssociateVPCWithHostedZone access denied"))
		},
		expectError: "failed to associate the Hosted Zone to the VPC vpc-hive1: api error AccessDenied: AssociateVPCWithHostedZone access denied",
	}, { // Desired VPCs should be associated if not already, and returned modified true
		name: "modified on association",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{},
			}, nil)
			m.EXPECT().AssociateVPCWithHostedZone(&route53.AssociateVPCWithHostedZoneInput{
				HostedZoneId: aws.String(testHostedZone),
				VPC: &route53types.VPC{
					VPCId:     aws.String(mockAWSPrivateLinkVPC.VPCID),
					VPCRegion: testRegion,
				},
			}).Return(nil, nil)
		},
		expect: true,
	}, { // There should be an error when DeleteVPCAssociationAuthorization fails when CredentialsSecretRef is different
		name: "CredentialsSecretRef, DeleteVPCAssociationAuthorization fails",
		cd:   cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC: mockAWSPrivateLinkVPC,
				CredentialsSecretRef: &corev1.LocalObjectReference{
					Name: "different",
				},
			}},
		},
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{},
			}, nil)
			m.EXPECT().CreateVPCAssociationAuthorization(gomock.Any()).Return(nil, nil)
			m.EXPECT().AssociateVPCWithHostedZone(gomock.Any()).Return(nil, nil)
			m.EXPECT().DeleteVPCAssociationAuthorization(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "DeleteVPCAssociationAuthorization access denied"))
		},
		expectError: "failed to delete authorization for association of the Hosted Zone to the VPC vpc-hive1: api error AccessDenied: DeleteVPCAssociationAuthorization access denied",
	}, { // There should be an error when failing to disassociate a VPC
		name: "failure disassociating VPC",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: testRegion,
				}, {
					VPCId:     aws.String("vpc-hive2"),
					VPCRegion: testRegion,
				}},
			}, nil)
			m.EXPECT().DisassociateVPCFromHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "DisassociateVPCFromHostedZone access denied"))
		},
		expectError: "failed to disassociate the Hosted Zone to the VPC vpc-hive2: api error AccessDenied: DisassociateVPCFromHostedZone access denied",
	}, { // Undesired VPCs should be removed, and returned modified true
		name: "modified on disassociation",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: testRegion,
				}, {
					VPCId:     aws.String("vpc-hive2"),
					VPCRegion: testRegion,
				}},
			}, nil)
			m.EXPECT().DisassociateVPCFromHostedZone(&route53.DisassociateVPCFromHostedZoneInput{
				HostedZoneId: aws.String(testHostedZone),
				VPC: &route53types.VPC{
					VPCId:     aws.String("vpc-hive2"),
					VPCRegion: testRegion,
				},
			},
			).Return(nil, nil)
		},
		expect: true,
	}, { // Modified false should be returned when no VPCS associated or disassociated
		name: "not modified",
		cd:   cdBuilder.Build(),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
				HostedZone: &route53types.HostedZone{
					Id: aws.String(testHostedZone),
				},
				VPCs: []route53types.VPC{{
					VPCId:     aws.String("vpc-hive1"),
					VPCRegion: testRegion,
				}},
			}, nil)
		},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs}
			}
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				test.config,
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.reconcileHostedZoneAssociations(test.cd, test.cd.Spec.ClusterMetadata, testHostedZone, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_getAssociatedVPCs(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		AWSClientConfig func(*mock.MockClient)

		expect      []hivev1.AWSAssociatedVPC
		expectError string
	}{{ //}, { There should be an error if VPCEndpointID is set the AWS API returns an error
		name: "VPCEndpointID exists but api error",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expect:      mockAssociatedVPCs,
		expectError: "error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // VPCEndpointID should be added to the result when it exists
		name: "VPCEndpointID exists",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(&ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []ec2types.VpcEndpoint{mockEndpoint},
			}, nil)
		},
		expect: append(mockAssociatedVPCs, hivev1.AWSAssociatedVPC{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  *mockEndpoint.VpcId,
				Region: string(testRegion),
			}}),
	}, { // The AssociatedVPCs should be returned
		name:   "success",
		cd:     cdBuilder.Build(),
		expect: mockAssociatedVPCs,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs},
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.getAssociatedVPCs(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_getEndpointVPC(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name string
		cd   *hivev1.ClusterDeployment

		AWSClientConfig func(*mock.MockClient)

		expect      hivev1.AWSAssociatedVPC
		expectError string
	}{{ // There should be an error returned when there is an error getting the VPC Endpoint
		name: "error getting vpc endpoint",
		cd:   cdBuilder.Build(testcd.WithClusterMetadata(&hivev1.ClusterMetadata{})),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expectError: "error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // The returned result should have the vpcid and the region
		name: "success",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(&ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []ec2types.VpcEndpoint{mockEndpoint},
			}, nil)
		},
		expect: hivev1.AWSAssociatedVPC{AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
			VPCID:  *mockEndpoint.VpcId,
			Region: string(testRegion),
		}},
	}}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				&hivev1.AWSPrivateLinkConfig{},
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.getEndpointVPC(test.cd, test.cd.Spec.ClusterMetadata)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

func Test_selectHostedZoneVPC(t *testing.T) {
	cdBuilder := testcd.FullBuilder(testNameSpace, testClusterDeployment, scheme.GetScheme())

	cases := []struct {
		name   string
		cd     *hivev1.ClusterDeployment
		config *hivev1.AWSPrivateLinkConfig

		AWSClientConfig func(*mock.MockClient)

		expect      hivev1.AWSAssociatedVPC
		expectError string
	}{{ // There should be an error if VPCEndpointID is set and getEndpointVPC fails
		name: "VPCEndpointID, getEndpointVPC failure",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(nil, awsclient.NewAPIError("AccessDenied", "not authorized to DescribeVpcEndpoints"))
		},
		expect:      hivev1.AWSAssociatedVPC{},
		expectError: "error getting Endpoint VPC: error getting the VPC Endpoint: api error AccessDenied: not authorized to DescribeVpcEndpoints",
	}, { // There should be an error if VPCEndPointID is set and getEndpointVPC returns an empty VPCID
		name: "VPCEndpointID, getEndpointVPC return empty vpcid",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(&ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []ec2types.VpcEndpoint{{VpcId: aws.String("")}},
			}, nil)
		},
		expect:      hivev1.AWSAssociatedVPC{},
		expectError: "unable to select Endpoint VPC: Endpoint not found",
	}, { // The AWS VPCEndpointID VPC should be used when set
		name: "VPCEndpointID, success",
		cd: cdBuilder.Build(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: string(testRegion)}),
			testcd.WithClusterMetadata(&hivev1.ClusterMetadata{InfraID: testInfraID}),
			testcd.WithAWSPlatformStatus(&hivev1aws.PlatformStatus{
				PrivateLink: &hivev1aws.PrivateLinkAccessStatus{
					VPCEndpointID: *mockEndpoint.VpcEndpointId,
				},
			}),
		),
		AWSClientConfig: func(m *mock.MockClient) {
			m.EXPECT().DescribeVpcEndpoints(gomock.Any()).Return(&ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []ec2types.VpcEndpoint{mockEndpoint},
			}, nil)
		},
		expect: hivev1.AWSAssociatedVPC{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  *mockEndpoint.VpcId,
				Region: string(testRegion),
			}},
		// There shuold be an error if getAssociatedVPCs fails. However, it can only fail when
		// VPCEndpointID is empty in which we will always return before getAssociatedVPCs is called.
	}, { // The first VPC should be returned that shares the same credentials due to not being defined
		name: "first vpc with credential null",
		cd:   cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "credential-1"},
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-1", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-2"},
			}, {
				AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{VPCID: "vpc-2", Region: string(testRegion)},
			}},
		},
		expect: hivev1.AWSAssociatedVPC{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-2",
				Region: string(testRegion),
			}},
	}, { // The first VPC should be returned that shares the same credentials due to matching secret
		name: "first vpc with credential matches",
		cd:   cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "credential-1"},
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-1", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-2"},
			}, {
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-2", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-1"},
			}},
		},
		expect: hivev1.AWSAssociatedVPC{
			AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
				VPCID:  "vpc-2",
				Region: string(testRegion),
			},
			CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-1"},
		},
	}, { // There should be an error if there are no associatedVPCS configured
		name:        "no associatedVPCS configured",
		cd:          cdBuilder.Build(),
		config:      &hivev1.AWSPrivateLinkConfig{},
		expectError: "unable to find an associatedVPC that uses the primary AWS PrivateLink credentials",
	}, { // There should be an error if no suitable associatedVPCS were configured
		name: "no suitable associatedVPCS configured",
		cd:   cdBuilder.Build(),
		config: &hivev1.AWSPrivateLinkConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{Name: "credential-1"},
			AssociatedVPCs: []hivev1.AWSAssociatedVPC{{
				AWSPrivateLinkVPC:    hivev1.AWSPrivateLinkVPC{VPCID: "vpc-1", Region: string(testRegion)},
				CredentialsSecretRef: &corev1.LocalObjectReference{Name: "credential-2"},
			}},
		},
		expectError: "unable to find an associatedVPC that uses the primary AWS PrivateLink credentials",
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.config == nil {
				test.config = &hivev1.AWSPrivateLinkConfig{AssociatedVPCs: mockAssociatedVPCs}
			}
			logger, _ := testlogger.NewLoggerWithHook()
			awsHubActuator, err := newTestAWSHubActuator(t,
				test.config,
				test.cd,
				[]runtime.Object{},
				test.AWSClientConfig,
				logger,
			)
			require.NoError(t, err)

			result, err := awsHubActuator.selectHostedZoneVPC(test.cd, test.cd.Spec.ClusterMetadata, logger)

			if test.expectError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, test.expectError)
			}

			assert.Equal(t, test.expect, result)
		})
	}
}

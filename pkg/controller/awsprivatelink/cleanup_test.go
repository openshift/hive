package awsprivatelink

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/awsclient/mock"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

func TestCleanupRequired(t *testing.T) {
	scheme := scheme.GetScheme()
	cdBuilder := testcd.FullBuilder(testNS, "test-cd", scheme)

	tests := []struct {
		name     string
		existing *hivev1.ClusterDeployment
		expected bool
	}{{
		name:     "PrivateLink is undefined",
		existing: cdBuilder.Build(testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1"})),
		expected: false,
	}, {
		name: "CD deleted with PreserveOnDelete enabled and PrivateLink enabled",
		existing: cdBuilder.GenericOptions(
			generic.Deleted(),
		).Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPreserveOnDelete(true),
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointID: "vpce-12345",
			}),
		),
		expected: false,
	}, {
		name: "CD deleted with PreserveOnDelete disabled and PrivateLink enabled",
		existing: cdBuilder.GenericOptions(
			generic.Deleted(),
		).Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPreserveOnDelete(false),
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointID: "vpce-12345",
			}),
		),
		expected: true,
	}, {
		name: "CD deleted with PreserveOnDelete enabled and PrivateLink disabled",
		existing: cdBuilder.GenericOptions(
			generic.Deleted(),
		).Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: false}}),
		).Build(
			withPreserveOnDelete(true),
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointID: "vpce-12345",
			}),
		),
		expected: true,
	}, {
		name: "PrivateLink is defined but empty",
		existing: cdBuilder.Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{Name: "", ID: ""},
				VPCEndpointID:      "",
				HostedZoneID:       "",
			}),
		),
		expected: false,
	}, {
		name: "VPCEndpointService.Name is not empty",
		existing: cdBuilder.Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{Name: "vpce-svc-12345.vpc.amazon.com", ID: ""},
			}),
		),
		expected: true,
	}, {
		name: "VPCEndpointService.ID is not empty",
		existing: cdBuilder.Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointService: hivev1aws.VPCEndpointService{Name: "", ID: "vpce-svc-12345"},
			}),
		),
		expected: true,
	}, {
		name: "VPCEndpointID is not empty",
		existing: cdBuilder.Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				VPCEndpointID: "vpce-12345",
			}),
		),
		expected: true,
	}, {
		name: "HostedZoneID is not empty",
		existing: cdBuilder.Options(
			testcd.WithAWSPlatform(&hivev1aws.Platform{Region: "us-east-1",
				PrivateLink: &hivev1aws.PrivateLinkAccess{Enabled: true}}),
		).Build(
			withPrivateLink(&hivev1aws.PrivateLinkAccessStatus{
				HostedZoneID: "HZ12345",
			}),
		),
		expected: true,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := cleanupRequired(test.existing)
			assert.EqualValues(t, result, test.expected)
		})
	}
}

func TestCleanupVPCEndpoints(t *testing.T) {
	tests := []struct {
		name          string
		metadata      *hivev1.ClusterMetadata
		mockSetup     func(*mock.MockClient)
		expectedError bool
	}{{
		name:     "no VPC endpoints found",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{},
				}, nil)
		},
		expectedError: false,
	}, {
		name:     "single VPC endpoint found and deleted successfully",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{{VpcEndpointId: aws.String("vpce-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpoints(&ec2.DeleteVpcEndpointsInput{
				VpcEndpointIds: aws.StringSlice([]string{"vpce-12345"}),
			}).Return(&ec2.DeleteVpcEndpointsOutput{}, nil)
		},
		expectedError: false,
	}, {
		name:     "multiple VPC endpoints found and deleted successfully",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{
						{VpcEndpointId: aws.String("vpce-12345")},
						{VpcEndpointId: aws.String("vpce-67890")},
						{VpcEndpointId: aws.String("vpce-abcde")},
					},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpoints(&ec2.DeleteVpcEndpointsInput{
				VpcEndpointIds: aws.StringSlice([]string{"vpce-12345", "vpce-67890", "vpce-abcde"}),
			}).Return(&ec2.DeleteVpcEndpointsOutput{}, nil)
		},
		expectedError: false,
	}, {
		name:     "AWS error on describe VPC endpoints",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(nil, awserr.New("InvalidParameter", "invalid parameter", nil))
		},
		expectedError: true,
	}, {
		name:     "AWS error on delete VPC endpoints (NotFound should be ignored)",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{{VpcEndpointId: aws.String("vpce-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpoints(gomock.Any()).
				Return(&ec2.DeleteVpcEndpointsOutput{}, awserr.New("InvalidVpcEndpointId.NotFound", "not found", nil))
		},
		expectedError: false,
	}, {
		name:     "AWS error on delete VPC endpoints (non-NotFound)",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []*ec2.VpcEndpoint{{VpcEndpointId: aws.String("vpce-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpoints(gomock.Any()).
				Return(&ec2.DeleteVpcEndpointsOutput{}, awserr.New("InvalidParameter", "invalid parameter", nil))
		},
		expectedError: true,
	}, {
		name:     "nil VpcEndpoints slice",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpoints(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: nil,
				}, nil)
		},
		expectedError: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAWSClient := mock.NewMockClient(ctrl)
			if test.mockSetup != nil {
				test.mockSetup(mockAWSClient)
			}

			reconciler := &ReconcileAWSPrivateLink{}
			logger := log.NewEntry(log.StandardLogger())

			err := reconciler.cleanupVPCEndpoints(mockAWSClient, test.metadata, logger)

			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCleanupVPCEndpointServices(t *testing.T) {
	tests := []struct {
		name          string
		metadata      *hivev1.ClusterMetadata
		mockSetup     func(*mock.MockClient)
		expectedError bool
	}{{
		name:     "no VPC endpoint services found",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{},
				}, nil)
		},
		expectedError: false,
	}, {
		name:     "single VPC endpoint service found and deleted successfully",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{{ServiceId: aws.String("vpce-svc-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpointServiceConfigurations(&ec2.DeleteVpcEndpointServiceConfigurationsInput{
				ServiceIds: aws.StringSlice([]string{"vpce-svc-12345"}),
			}).Return(&ec2.DeleteVpcEndpointServiceConfigurationsOutput{}, nil)
		},
		expectedError: false,
	}, {
		name:     "multiple VPC endpoint services found and deleted successfully",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{
						{ServiceId: aws.String("vpce-svc-12345")},
						{ServiceId: aws.String("vpce-svc-67890")},
						{ServiceId: aws.String("vpce-svc-abcde")},
					},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpointServiceConfigurations(&ec2.DeleteVpcEndpointServiceConfigurationsInput{
				ServiceIds: aws.StringSlice([]string{"vpce-svc-12345", "vpce-svc-67890", "vpce-svc-abcde"}),
			}).Return(&ec2.DeleteVpcEndpointServiceConfigurationsOutput{}, nil)
		},
		expectedError: false,
	}, {
		name:     "AWS error on describe VPC endpoint services",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(nil, awserr.New("InvalidParameter", "invalid parameter", nil))
		},
		expectedError: true,
	}, {
		name:     "AWS error on delete VPC endpoint services (NotFound should be ignored)",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{{ServiceId: aws.String("vpce-svc-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DeleteVpcEndpointServiceConfigurationsOutput{}, awserr.New("InvalidVpcEndpointService.NotFound", "not found", nil))
		},
		expectedError: false,
	}, {
		name:     "AWS error on delete VPC endpoint services (non-NotFound)",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: []*ec2.ServiceConfiguration{{ServiceId: aws.String("vpce-svc-12345")}},
				}, nil)
			mockClient.EXPECT().DeleteVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DeleteVpcEndpointServiceConfigurationsOutput{}, awserr.New("InvalidParameter", "invalid parameter", nil))
		},
		expectedError: true,
	}, {
		name:     "nil ServiceConfigurations slice",
		metadata: &hivev1.ClusterMetadata{InfraID: "test-infra-123"},
		mockSetup: func(mockClient *mock.MockClient) {
			mockClient.EXPECT().DescribeVpcEndpointServiceConfigurations(gomock.Any()).
				Return(&ec2.DescribeVpcEndpointServiceConfigurationsOutput{
					ServiceConfigurations: nil,
				}, nil)
		},
		expectedError: false,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockAWSClient := mock.NewMockClient(ctrl)
			if test.mockSetup != nil {
				test.mockSetup(mockAWSClient)
			}

			reconciler := &ReconcileAWSPrivateLink{}
			logger := log.NewEntry(log.StandardLogger())

			err := reconciler.cleanupVPCEndpointServices(mockAWSClient, test.metadata, logger)

			if test.expectedError {
				assert.Error(t, err)
			} else {

				assert.NoError(t, err)
			}
		})
	}
}

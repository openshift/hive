package awsprivatelink

import (
	"testing"

	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
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

package nameserver

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"go.uber.org/mock/gomock"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/awsclient/mock"
)

func TestAWSGet(t *testing.T) {
	cases := []struct {
		name                          string
		listHostedZonesOutputs        []*route53.ListHostedZonesByNameOutput
		listResourceRecordSetsOutputs []*route53.ListResourceRecordSetsOutput
		expectedNameServers           map[string]sets.Set[string]
	}{
		{
			name: "no hosted zones",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(),
			},
		},
		{
			name: "no hosted zones for domain",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(hzTruncated(), withHostedZones(testHostedZone("other-domain.", "other-zone-id"))),
			},
		},
		{
			name: "no public hosted zones for domain",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id", private()))),
			},
		},
		{
			name: "public and private hosted zones for domain",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(
					hzTruncated(),
					withHostedZones(
						testHostedZone("test-domain.", "other-zone-id", private()),
						testHostedZone("test-domain.", "test-zone-id"),
					),
				),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New("test-ns"),
			},
		},
		{
			name: "public hosted zone in second list",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(
					hzTruncated(),
					withHostedZones(testHostedZone("test-domain.", "other-zone-id", private())),
				),
				testListHostedZonesOutput(
					hzTruncated(),
					withHostedZones(testHostedZone("test-domain.", "test-zone-id")),
				),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New("test-ns"),
			},
		},
		{
			name: "no records",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(),
			},
		},
		{
			name: "no name server records",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain.", "A", "test-ns"),
				)),
			},
		},
		{
			name: "single name server",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New("test-ns"),
			},
		},
		{
			name: "multiple name servers for domain",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain.", "NS", "test-ns-1", "test-ns-2", "test-ns-3"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New("test-ns-1", "test-ns-2", "test-ns-3"),
			},
		},
		{
			name: "name servers for multiple domains",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(withRecordSets(
					testRecordSet("test-subdomain-1.", "NS", "test-ns-1"),
					testRecordSet("test-subdomain-2.", "NS", "test-ns-2"),
					testRecordSet("test-subdomain-3.", "NS", "test-ns-3"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain-1": sets.New("test-ns-1"),
				"test-subdomain-2": sets.New("test-ns-2"),
				"test-subdomain-3": sets.New("test-ns-3"),
			},
		},
		{
			name: "multiple record sets",
			listHostedZonesOutputs: []*route53.ListHostedZonesByNameOutput{
				testListHostedZonesOutput(withHostedZones(testHostedZone("test-domain.", "test-zone-id"))),
			},
			listResourceRecordSetsOutputs: []*route53.ListResourceRecordSetsOutput{
				testListResourceRecordSetsOutput(
					rrsTruncated(),
					withRecordSets(
						testRecordSet("test-subdomain-1.", "NS", "test-ns-1"),
						testRecordSet("other-subdomain.", "A", "other-value"),
					),
				),
				testListResourceRecordSetsOutput(
					withRecordSets(
						testRecordSet("test-subdomain-2.", "NS", "test-ns-2"),
					),
				),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain-1": sets.New("test-ns-1"),
				"test-subdomain-2": sets.New("test-ns-2"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockAWSClient := mock.NewMockClient(mockCtrl)
			awsQuery := &awsQuery{
				getAWSClient: func() (awsclient.Client, error) {
					return mockAWSClient, nil
				},
			}
			for i, out := range tc.listHostedZonesOutputs {
				in := &route53.ListHostedZonesByNameInput{
					DNSName:  aws.String("test-domain."),
					MaxItems: aws.Int32(5),
				}
				if i > 0 {
					in.DNSName = aws.String("next-dns-name")
					in.HostedZoneId = aws.String("next-hosted-zone-id")
				}
				mockAWSClient.EXPECT().
					ListHostedZonesByName(gomock.Eq(in)).
					Return(out, nil)
			}
			for i, out := range tc.listResourceRecordSetsOutputs {
				in := &route53.ListResourceRecordSetsInput{
					HostedZoneId: aws.String("test-zone-id"),
					MaxItems:     aws.Int32(100),
				}
				if i > 0 {
					in.StartRecordName = aws.String("next-record-name")
					in.StartRecordType = route53types.RRTypeA
				}
				mockAWSClient.EXPECT().
					ListResourceRecordSets(gomock.Eq(in)).
					Return(out, nil)
			}
			actualNameServers, err := awsQuery.Get("test-domain")
			assert.NoError(t, err, "expected no error from querying")
			if len(tc.expectedNameServers) == 0 {
				assert.Empty(t, actualNameServers, "expected no name servers")
			} else {
				assert.Equal(t, tc.expectedNameServers, actualNameServers, "unexpected name servers")
			}
		})
	}
}

type listHostedZonesOutputOption func(*route53.ListHostedZonesByNameOutput)

func testListHostedZonesOutput(opts ...listHostedZonesOutputOption) *route53.ListHostedZonesByNameOutput {
	out := &route53.ListHostedZonesByNameOutput{}
	for _, o := range opts {
		o(out)
	}
	return out
}

func withHostedZones(hostedZones ...route53types.HostedZone) listHostedZonesOutputOption {
	return func(out *route53.ListHostedZonesByNameOutput) {
		out.HostedZones = hostedZones
	}
}

func hzTruncated() listHostedZonesOutputOption {
	return func(out *route53.ListHostedZonesByNameOutput) {
		out.IsTruncated = true
		out.NextDNSName = aws.String("next-dns-name")
		out.NextHostedZoneId = aws.String("next-hosted-zone-id")
	}
}

type hostedZoneOption func(route53types.HostedZone)

func testHostedZone(name string, zoneID string, opts ...hostedZoneOption) route53types.HostedZone {
	zone := route53types.HostedZone{
		Name: &name,
		Id:   &zoneID,
		Config: &route53types.HostedZoneConfig{
			PrivateZone: false,
		},
	}
	for _, o := range opts {
		o(zone)
	}
	return zone
}

func private() hostedZoneOption {
	return func(zone route53types.HostedZone) {
		zone.Config.PrivateZone = true
	}
}

type listResourceRecordSetsOutputOption func(*route53.ListResourceRecordSetsOutput)

func testListResourceRecordSetsOutput(opts ...listResourceRecordSetsOutputOption) *route53.ListResourceRecordSetsOutput {
	out := &route53.ListResourceRecordSetsOutput{}
	for _, o := range opts {
		o(out)
	}
	return out
}

func withRecordSets(recordSets ...route53types.ResourceRecordSet) listResourceRecordSetsOutputOption {
	return func(out *route53.ListResourceRecordSetsOutput) {
		out.ResourceRecordSets = recordSets
	}
}

func rrsTruncated() listResourceRecordSetsOutputOption {
	return func(out *route53.ListResourceRecordSetsOutput) {
		out.IsTruncated = true
		out.NextRecordName = aws.String("next-record-name")
		out.NextRecordType = route53types.RRTypeA
	}
}

func testRecordSet(name string, recordType route53types.RRType, values ...string) route53types.ResourceRecordSet {
	recordSet := route53types.ResourceRecordSet{
		Name:            &name,
		Type:            recordType,
		ResourceRecords: make([]route53types.ResourceRecord, len(values)),
	}
	for i, value := range values {
		recordSet.ResourceRecords[i] = route53types.ResourceRecord{
			Value: aws.String(value),
		}
	}
	return recordSet
}

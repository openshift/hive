package nameserver

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	dns "google.golang.org/api/dns/v1"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/pkg/gcpclient"
	"github.com/openshift/hive/pkg/gcpclient/mock"
)

func TestGCPGet(t *testing.T) {
	cases := []struct {
		name                            string
		listManagedZonesResponses       []*dns.ManagedZonesListResponse
		listResourceRecordSetsResponses []*dns.ResourceRecordSetsListResponse
		expectedNameServers             map[string]sets.Set[string]
	}{
		{
			name: "no managed zones",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(),
			},
		},
		{
			name: "no public managed zones for domain",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name", gcp.private()))),
			},
		},
		{
			name: "public and private managed zones for domain",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(
					gcp.mzTruncated(),
					gcp.withManagedZones(
						gcp.managedZone("test-domain.", "other-zone-name", gcp.private()),
						gcp.managedZone("test-domain.", "test-zone-name"),
					),
				),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New[string]("test-ns"),
			},
		},
		{
			name: "public managed zone in second list",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(
					gcp.mzTruncated(),
					gcp.withManagedZones(gcp.managedZone("test-domain.", "other-zone-name", gcp.private())),
				),
				gcp.listManagedZonesResponse(
					gcp.mzTruncated(),
					gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name")),
				),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New[string]("test-ns"),
			},
		},
		{
			name: "no records",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(),
			},
		},
		{
			name: "no name server records",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain.", "A", "test-ns"),
				)),
			},
		},
		{
			name: "single name server",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain.", "NS", "test-ns"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New[string]("test-ns"),
			},
		},
		{
			name: "multiple name servers for domain",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain.", "NS", "test-ns-1", "test-ns-2", "test-ns-3"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain": sets.New[string]("test-ns-1", "test-ns-2", "test-ns-3"),
			},
		},
		{
			name: "name servers for multiple domains",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(gcp.withRecordSets(
					gcp.recordSet("test-subdomain-1.", "NS", "test-ns-1"),
					gcp.recordSet("test-subdomain-2.", "NS", "test-ns-2"),
					gcp.recordSet("test-subdomain-3.", "NS", "test-ns-3"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain-1": sets.New[string]("test-ns-1"),
				"test-subdomain-2": sets.New[string]("test-ns-2"),
				"test-subdomain-3": sets.New[string]("test-ns-3"),
			},
		},
		{
			name: "multiple record sets",
			listManagedZonesResponses: []*dns.ManagedZonesListResponse{
				gcp.listManagedZonesResponse(gcp.withManagedZones(gcp.managedZone("test-domain.", "test-zone-name"))),
			},
			listResourceRecordSetsResponses: []*dns.ResourceRecordSetsListResponse{
				gcp.listResourceRecordSetsResponse(
					gcp.rrsTruncated(),
					gcp.withRecordSets(
						gcp.recordSet("test-subdomain-1.", "NS", "test-ns-1"),
						gcp.recordSet("other-subdomain.", "A", "other-value"),
					),
				),
				gcp.listResourceRecordSetsResponse(
					gcp.withRecordSets(
						gcp.recordSet("test-subdomain-2.", "NS", "test-ns-2"),
					),
				),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain-1": sets.New[string]("test-ns-1"),
				"test-subdomain-2": sets.New[string]("test-ns-2"),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockGCPClient := mock.NewMockClient(mockCtrl)
			gcpQuery := &gcpQuery{
				getGCPClient: func() (gcpclient.Client, error) {
					return mockGCPClient, nil
				},
			}
			for i, out := range tc.listManagedZonesResponses {
				opts := gcpclient.ListManagedZonesOptions{
					DNSName: "test-domain.",
				}
				if i > 0 {
					opts.PageToken = "next-page-token"
				}
				mockGCPClient.EXPECT().
					ListManagedZones(gomock.Eq(opts)).
					Return(out, nil)
			}
			for i, out := range tc.listResourceRecordSetsResponses {
				opts := gcpclient.ListResourceRecordSetsOptions{}
				if i > 0 {
					opts.PageToken = "next-page-token"
				}
				mockGCPClient.EXPECT().
					ListResourceRecordSets(gomock.Eq("test-zone-name"), gomock.Eq(opts)).
					Return(out, nil)
			}
			actualNameServers, err := gcpQuery.Get("test-domain")
			assert.NoError(t, err, "expected no error from querying")
			if len(tc.expectedNameServers) == 0 {
				assert.Empty(t, actualNameServers, "expected no name servers")
			} else {
				assert.Equal(t, tc.expectedNameServers, actualNameServers, "unexpected name servers")
			}
		})
	}
}

type gcpTestFuncs struct{}

var gcp gcpTestFuncs

type gcpListManagedZonesResponseOption func(*dns.ManagedZonesListResponse)

func (*gcpTestFuncs) listManagedZonesResponse(opts ...gcpListManagedZonesResponseOption) *dns.ManagedZonesListResponse {
	out := &dns.ManagedZonesListResponse{}
	for _, o := range opts {
		o(out)
	}
	return out
}

func (*gcpTestFuncs) withManagedZones(managedZones ...*dns.ManagedZone) gcpListManagedZonesResponseOption {
	return func(out *dns.ManagedZonesListResponse) {
		out.ManagedZones = managedZones
	}
}

func (*gcpTestFuncs) mzTruncated() gcpListManagedZonesResponseOption {
	return func(out *dns.ManagedZonesListResponse) {
		out.NextPageToken = "next-page-token"
	}
}

type gcpManagedZoneOption func(*dns.ManagedZone)

func (*gcpTestFuncs) managedZone(domain string, name string, opts ...gcpManagedZoneOption) *dns.ManagedZone {
	zone := &dns.ManagedZone{
		DnsName:    domain,
		Name:       name,
		Visibility: "public",
	}
	for _, o := range opts {
		o(zone)
	}
	return zone
}

func (*gcpTestFuncs) private() gcpManagedZoneOption {
	return func(zone *dns.ManagedZone) {
		zone.Visibility = "private"
	}
}

type gcpListResourceRecordSetsResponseOption func(*dns.ResourceRecordSetsListResponse)

func (*gcpTestFuncs) listResourceRecordSetsResponse(opts ...gcpListResourceRecordSetsResponseOption) *dns.ResourceRecordSetsListResponse {
	out := &dns.ResourceRecordSetsListResponse{}
	for _, o := range opts {
		o(out)
	}
	return out
}

func (*gcpTestFuncs) withRecordSets(recordSets ...*dns.ResourceRecordSet) gcpListResourceRecordSetsResponseOption {
	return func(out *dns.ResourceRecordSetsListResponse) {
		out.Rrsets = recordSets
	}
}

func (*gcpTestFuncs) rrsTruncated() gcpListResourceRecordSetsResponseOption {
	return func(out *dns.ResourceRecordSetsListResponse) {
		out.NextPageToken = "next-page-token"
	}
}

func (*gcpTestFuncs) recordSet(name string, recordType string, values ...string) *dns.ResourceRecordSet {
	return &dns.ResourceRecordSet{
		Name:    name,
		Type:    recordType,
		Rrdatas: values,
	}
}

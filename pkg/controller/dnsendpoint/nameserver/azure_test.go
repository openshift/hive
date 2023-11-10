package nameserver

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/hive/pkg/azureclient"
	"github.com/openshift/hive/pkg/azureclient/mock"
)

type RecordSetPage struct {
	Results []dns.RecordSet
}

func TestAzureGet(t *testing.T) {
	cases := []struct {
		name                string
		listRecordSets      []*RecordSetPage
		expectedNameServers map[string]sets.Set[string]
	}{
		{
			name: "single name server",
			listRecordSets: []*RecordSetPage{
				azure.recordSetPage(azure.withRecordSets(azure.recordSet("test-subdomain", "NS", "test-ns"))),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain.test-domain": sets.New[string]("test-ns"),
			},
		},
		{
			name: "no records",
			listRecordSets: []*RecordSetPage{
				azure.recordSetPage(),
			},
		},
		{
			name: "no name server records",
			listRecordSets: []*RecordSetPage{
				azure.recordSetPage(azure.withRecordSets(azure.recordSet("test-subdomain", "A", "test-ns"))),
			},
		},
		{
			name: "multiple name servers",
			listRecordSets: []*RecordSetPage{
				azure.recordSetPage(azure.withRecordSets(azure.recordSet("test-subdomain", "NS", "test-ns-1", "test-ns-2", "test-ns-3"))),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain.test-domain": sets.New[string]("test-ns-1", "test-ns-2", "test-ns-3"),
			},
		},
		{
			name: "name servers for multiple domains",
			listRecordSets: []*RecordSetPage{
				azure.recordSetPage(azure.withRecordSets(
					azure.recordSet("test-subdomain-1", "NS", "test-ns-1"),
					azure.recordSet("test-subdomain-2", "NS", "test-ns-2"),
					azure.recordSet("test-subdomain-3", "NS", "test-ns-3"),
				)),
			},
			expectedNameServers: map[string]sets.Set[string]{
				"test-subdomain-1.test-domain": sets.New[string]("test-ns-1"),
				"test-subdomain-2.test-domain": sets.New[string]("test-ns-2"),
				"test-subdomain-3.test-domain": sets.New[string]("test-ns-3"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockAzureClient := mock.NewMockClient(mockCtrl)

			azureQuery := &azureQuery{
				getAzureClient: func() (azureclient.Client, error) {
					return mockAzureClient, nil
				},
				resourceGroupName: "default",
			}

			for _, out := range tc.listRecordSets {
				mockListRecordSets(mockCtrl, mockAzureClient, out)
			}

			actualNameservers, err := azureQuery.Get("test-domain")
			assert.NoError(t, err, "expected no error from querying")
			if len(tc.expectedNameServers) == 0 {
				assert.Empty(t, actualNameservers, "expected no name servers")
			} else {
				assert.Equal(t, tc.expectedNameServers, actualNameservers, "unexpected name servers")
			}
		})
	}
}

func mockListRecordSets(mockCtrl *gomock.Controller, client *mock.MockClient, recordSetPage *RecordSetPage) {
	page := mock.NewMockRecordSetPage(mockCtrl)
	client.EXPECT().ListRecordSetsByZone(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(page, nil)
	page.EXPECT().NotDone().Return(false).Times(1).After(page.EXPECT().NotDone().Return(true).Times(1))

	page.EXPECT().NextWithContext(gomock.Any()).Return(nil)
	page.EXPECT().Values().Return(recordSetPage.Results)
}

type azureTestFuncs struct{}

var azure azureTestFuncs

type azureRecordSetPageOption func(*RecordSetPage)

func (*azureTestFuncs) recordSetPage(opts ...azureRecordSetPageOption) *RecordSetPage {
	out := &RecordSetPage{}
	for _, o := range opts {
		o(out)
	}
	return out
}

func (*azureTestFuncs) withRecordSets(recordSets ...dns.RecordSet) azureRecordSetPageOption {
	return func(out *RecordSetPage) {
		out.Results = recordSets
	}
}

func (*azureTestFuncs) recordSet(name string, rType string, values ...string) dns.RecordSet {
	var recordSetProperties dns.RecordSetProperties
	switch rType {
	case "NS":
		var nsRecords []dns.NsRecord
		for _, value := range values {
			nsRecords = append(nsRecords, dns.NsRecord{
				Nsdname: to.StringPtr(value),
			})
		}
		recordSetProperties = dns.RecordSetProperties{
			NsRecords: &nsRecords,
		}
	case "A":
		var aRecords []dns.ARecord
		for _, value := range values {
			aRecords = append(aRecords, dns.ARecord{
				Ipv4Address: to.StringPtr(value),
			})
		}
		recordSetProperties = dns.RecordSetProperties{
			ARecords: &aRecords,
		}
	}

	return dns.RecordSet{
		Name:                &name,
		Type:                &rType,
		RecordSetProperties: &recordSetProperties,
	}
}

package dnsendpoint

import (
	"context"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock"
)

func TestGetEndpoint(t *testing.T) {
	rootDomain := "domain.com"
	domain := "test.domain.com"
	cases := []struct {
		name             string
		nameServers      rootDomainsMap
		expectRootDomain bool
		expectedValues   sets.Set[string]
	}{
		{
			name:           "empty",
			expectedValues: sets.Set[string]{},
		},
		{
			name: "no root domain",
			nameServers: rootDomainsMap{
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectedValues: sets.Set[string]{},
		},
		{
			name: "empty root domain",
			nameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectRootDomain: true,
			expectedValues:   sets.Set[string]{},
		},
		{
			name: "no domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
			},
			expectRootDomain: true,
			expectedValues:   sets.Set[string]{},
		},
		{
			name: "single namespace value",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
			expectRootDomain: true,
			expectedValues:   sets.New("test-value"),
		},
		{
			name: "multiple namespace values",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							nsValues: sets.New("test-value-1", "test-value-2", "test-value-3"),
						},
					},
				},
			},
			expectRootDomain: true,
			expectedValues:   sets.New("test-value-1", "test-value-2", "test-value-3"),
		},
		{
			name: "many root domains and domains",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							nsValues: sets.New("test-value"),
						},
						"other.domain.com": endpointState{
							nsValues: sets.New("other-value"),
						},
					},
				},
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"sub-domain.other-domain": endpointState{
							nsValues: sets.New("another-value"),
						},
					},
				},
			},
			expectRootDomain: true,
			expectedValues:   sets.New("test-value"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := &nameServerScraper{rootDomainsMap: tc.nameServers}
			actualRootDomain, actualValues := cut.GetEndpoint("test.domain.com")
			if tc.expectRootDomain {
				assert.Equal(t, "domain.com", actualRootDomain, "unexpected root domain")
			} else {
				assert.Empty(t, actualRootDomain, "expected to not find root domain")
			}
			assert.Equal(t, tc.expectedValues, actualValues, "unexpected name server values")
		})
	}
}

func TestAddEndpoint(t *testing.T) {
	rootDomain := "domain.com"
	domain := "test.domain.com"
	values := sets.New("test-value-1", "test-value-2", "test-value-3")
	cases := []struct {
		name                string
		nameServers         rootDomainsMap
		expectedNameServers rootDomainsMap
	}{
		{
			name: "empty",
		},
		{
			name: "no root domain",
			nameServers: rootDomainsMap{
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectedNameServers: rootDomainsMap{
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
		},
		{
			name: "empty root domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: values,
						},
					},
				},
			},
		},
		{
			name: "no domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: values,
						},
					},
				},
			},
		},
		{
			name: "update domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							dnsZone: func() *hivev1.DNSZone {
								dz := testDNSZone()
								dz.Name = "other-name"
								dz.Namespace = "other-namespace"
								return dz
							}(),
							nsValues: sets.New("other-value"),
						},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: values,
						},
					},
				},
			},
		},
		{
			name: "multiple root domains",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"subdomain.other-domain": endpointState{},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: values,
						},
					},
				},
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"subdomain.other-domain": endpointState{},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := &nameServerScraper{rootDomainsMap: tc.nameServers}
			cut.SyncEndpoint(testDNSZone(), domain, values)
			assert.Equal(t, tc.expectedNameServers, cut.rootDomainsMap, "unexpected changes to name servers")
		})
	}
}

func TestRemoveEndpoint(t *testing.T) {
	rootDomain := "domain.com"
	domain := "test.domain.com"
	cases := []struct {
		name                string
		nameServers         rootDomainsMap
		expectedNameServers rootDomainsMap
	}{
		{
			name: "empty",
		},
		{
			name: "no root domain",
			nameServers: rootDomainsMap{
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectedNameServers: rootDomainsMap{
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
		},
		{
			name: "empty root domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
		},
		{
			name: "no domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
			},
		},
		{
			name: "remove domain",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
		},
		{
			name: "multiple root domains",
			nameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
						domain: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("test-value"),
						},
					},
				},
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"subdomain.other-domain": endpointState{},
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"other.domain.com": endpointState{},
					},
				},
				"other-domain": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"subdomain.other-domain": endpointState{},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cut := &nameServerScraper{rootDomainsMap: tc.nameServers}
			cut.RemoveEndpoint(domain)
			assert.Equal(t, tc.expectedNameServers, cut.rootDomainsMap, "unexpected changes to name servers")
		})
	}
}

func TestCheckSeedScrapeStatus(t *testing.T) {
	cases := []struct {
		name         string
		cache        rootDomainsMap
		expectReturn bool
		expectSeeded bool
	}{
		{
			name: "error path: empty",
		},
		{
			name: "error path: unregistered rootDomain",
			cache: rootDomainsMap{
				"other.com": &rootDomainsInfo{},
			},
		},
		{
			name: "not scraped, not seeded",
			cache: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					scraped:              false,
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectSeeded: true,
		},
		{
			name: "scraped but not seeded",
			cache: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					scraped:              true,
					endpointsBySubdomain: endpointsBySubdomain{},
				},
			},
			expectReturn: true,
			expectSeeded: true,
		},
		{
			name: "seeded but not scraped",
			cache: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					scraped: false,
					endpointsBySubdomain: endpointsBySubdomain{
						dnsName: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.Set[string]{},
						},
					},
				},
			},
			expectSeeded: true,
		},
		{
			name: "seeded and scraped",
			cache: rootDomainsMap{
				rootDomain: &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						dnsName: endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.Set[string]{},
						},
					},
				},
			},
			expectReturn: true,
			expectSeeded: true,
		},
	}
	rootDomains := []string{rootDomain}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			zone := testDNSZone()
			scraper := newNameServerScraper(log.StandardLogger(), nil, rootDomains, nil)
			if tc.cache != nil {
				scraper.rootDomainsMap = tc.cache
			}
			actualReturn := scraper.CheckSeedScrapeStatus(zone, rootDomain)
			assert.Equal(t, tc.expectReturn, actualReturn, "unexpected return from CheckSeedScrapeStatus")
			rdInfo, ok := scraper.rootDomainsMap[rootDomain]
			if tc.expectSeeded {
				// We can only check the guts if we didn't fail
				if assert.True(t, ok, "rootDomainsMap didn't contain rootDomain %s", rootDomain) &&
					assert.NotNil(t, rdInfo, "nil root domain info for rootDomain %s", rootDomain) {
					assert.Equal(t, tc.expectReturn, rdInfo.scraped, "expected return didn't match internal scrape value")
					if assert.Contains(t, rdInfo.endpointsBySubdomain, dnsName, "expected cache to be seeded with zone %s", dnsName) {
						assert.Equal(t, zone, rdInfo.endpointsBySubdomain[dnsName].dnsZone, "mismatched DNSZone in cache")
					}
				}
			}
		})
	}
}

func TestScrape(t *testing.T) {
	cases := []struct {
		name                string
		testDuration        time.Duration
		scrapePeriod        time.Duration
		rootDomains         []string
		nameServers         rootDomainsMap
		configureQuery      func(mockQuery *mock.MockQuery)
		expectedNameServers rootDomainsMap
		expectedChanges     []client.ObjectKey
	}{
		{
			name:        "single root domain",
			rootDomains: []string{"domain.com"},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").Return(nil, nil)
			},
		},
		{
			name:        "multiple root domains",
			rootDomains: []string{"domain1.com", "domain2.com", "domain3.com"},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain1.com").Return(nil, nil)
				mockQuery.EXPECT().Get("domain2.com").Return(nil, nil)
				mockQuery.EXPECT().Get("domain3.com").Return(nil, nil)
			},
		},
		{
			name:         "scrape after scrape period",
			scrapePeriod: 2 * time.Second,
			testDuration: 3 * time.Second,
			rootDomains:  []string{"domain1.com", "domain2.com"},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain1.com").Return(nil, nil).Times(2)
				mockQuery.EXPECT().Get("domain2.com").Return(nil, nil).Times(2)
			},
		},
		{
			name:         "scrape after error",
			testDuration: 12 * time.Second,
			rootDomains:  []string{"domain1.com", "domain2.com"},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain1.com").Return(nil, errors.New("test error"))
				mockQuery.EXPECT().Get("domain1.com").Return(nil, nil)
				mockQuery.EXPECT().Get("domain2.com").Return(nil, nil)
			},
		},
		{
			name:        "untracked domain",
			rootDomains: []string{"domain.com"},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").
					Return(
						map[string]sets.Set[string]{
							"test.domain.com": sets.New("test-value"),
						},
						nil,
					)
			},
			expectedNameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  nil,
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
		},
		{
			name:        "update domain",
			rootDomains: []string{"domain.com"},
			nameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("old-value"),
						},
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").
					Return(
						map[string]sets.Set[string]{
							"test.domain.com": sets.New("test-value"),
						},
						nil,
					)
			},
			expectedNameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
			expectedChanges: []client.ObjectKey{{Namespace: "test-namespace", Name: "test-name"}},
		},
		{
			name:        "no changes to domain",
			rootDomains: []string{"domain.com"},
			nameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").
					Return(
						map[string]sets.Set[string]{
							"test.domain.com": sets.New("test-value"),
						},
						nil,
					)
			},
			expectedNameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.New("test-value"),
						},
					},
				},
			},
		},
		{
			name:        "update multiple domains",
			rootDomains: []string{"domain.com"},
			nameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"changed-1.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-1"),
							nsValues: sets.New("old-value-1"),
						},
						"changed-2.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-2"),
							nsValues: sets.New("old-value-2"),
						},
						"changed-3.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-3"),
							nsValues: sets.New("old-value-3a", "old-value-3b"),
						},
						"unchanged.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-unchanged"),
							nsValues: sets.New("test-value-4"),
						},
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").
					Return(
						map[string]sets.Set[string]{
							"changed-1.domain.com": sets.New("test-value-1"),
							"changed-2.domain.com": sets.New("test-value-2a", "test-value-2b"),
							"changed-3.domain.com": sets.New("test-value-3"),
							"unchanged.domain.com": sets.New("test-value-4"),
							"untracked.domain.com": sets.New("test-value-5"),
						},
						nil,
					)
			},
			expectedNameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						"changed-1.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-1"),
							nsValues: sets.New("test-value-1"),
						},
						"changed-2.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-2"),
							nsValues: sets.New("test-value-2a", "test-value-2b"),
						},
						"changed-3.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-changed-3"),
							nsValues: sets.New("test-value-3"),
						},
						"unchanged.domain.com": endpointState{
							dnsZone:  testDNSZoneWithNSName(testNamespace, "test-unchanged"),
							nsValues: sets.New("test-value-4"),
						},
						"untracked.domain.com": endpointState{
							dnsZone:  nil,
							nsValues: sets.New("test-value-5"),
						},
					},
				},
			},
			expectedChanges: []client.ObjectKey{
				{Namespace: "test-namespace", Name: "test-changed-1"},
				{Namespace: "test-namespace", Name: "test-changed-2"},
				{Namespace: "test-namespace", Name: "test-changed-3"},
			},
		},
		{
			name:        "notify on new, seeded domain with no NS entries yet",
			rootDomains: []string{"domain.com"},
			nameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.Set[string]{},
						},
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Get("domain.com").
					Return(
						map[string]sets.Set[string]{},
						nil,
					)
			},
			expectedNameServers: rootDomainsMap{
				"domain.com": &rootDomainsInfo{
					scraped: true,
					endpointsBySubdomain: endpointsBySubdomain{
						"test.domain.com": endpointState{
							dnsZone:  testDNSZone(),
							nsValues: sets.Set[string]{},
						},
					},
				},
			},
			expectedChanges: []client.ObjectKey{{Namespace: "test-namespace", Name: "test-name"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockQuery := mock.NewMockQuery(mockCtrl)
			tc.configureQuery(mockQuery)
			changeNotifications := make(chan client.Object, 100)
			notifyChange := func(object client.Object) {
				changeNotifications <- object
			}
			cut := newNameServerScraper(log.StandardLogger(), mockQuery, tc.rootDomains, notifyChange)
			if tc.scrapePeriod > 0 {
				cut.scrapePeriod = tc.scrapePeriod
			}
			if tc.nameServers != nil {
				cut.rootDomainsMap = tc.nameServers
			}
			ctx, stop := context.WithCancel(context.Background())
			go func() {
				sleepTime := tc.testDuration
				if sleepTime <= 0 {
					sleepTime = time.Second
				}
				time.Sleep(sleepTime)
				stop()
			}()
			err := cut.Start(ctx)
			assert.NoError(t, err, "unexpected error starting scraper")
			expectedNameServers := tc.expectedNameServers
			if len(expectedNameServers) == 0 {
				expectedNameServers = make(rootDomainsMap, len(tc.rootDomains))
				for _, d := range tc.rootDomains {
					expectedNameServers[d] = &rootDomainsInfo{
						scraped:              true,
						endpointsBySubdomain: endpointsBySubdomain{},
					}
				}
			}
			assert.Equal(t, expectedNameServers, cut.rootDomainsMap, "unexpected changes to name servers")
			actualChanges := []client.Object{}
			for {
				empty := false
				select {
				case key := <-changeNotifications:
					actualChanges = append(actualChanges, key)
				default:
					empty = true
				}
				if empty {
					break
				}
			}
			if len(tc.expectedChanges) == 0 {
				assert.Empty(t, actualChanges, "expected no change notifications")
			} else {
				assert.Equal(t, len(tc.expectedChanges), len(actualChanges), "unexpected change count")
				for _, expectedChangedDNSZone := range tc.expectedChanges {
					found := false
					for _, actualChangedDNSZone := range actualChanges {
						if actualChangedDNSZone.GetNamespace() == expectedChangedDNSZone.Namespace &&
							actualChangedDNSZone.GetName() == expectedChangedDNSZone.Name {
							found = true
						}
					}
					assert.True(t, found, "expected change to DNSZone %s did not occur", expectedChangedDNSZone.Name)
				}
			}
		})
	}
}

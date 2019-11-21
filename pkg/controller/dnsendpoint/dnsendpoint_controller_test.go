package dnsendpoint

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock"
)

const (
	testNamespace = "test-namespace"
	testName      = "test-name"
	rootDomain    = "domain.com"
	dnsName       = "test.domain.com"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestDNSEndpointReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	objectKey := client.ObjectKey{Namespace: testNamespace, Name: testName}

	cases := []struct {
		name                string
		dnsEndpoint         *hivev1.DNSEndpoint
		nameServers         rootDomainsMap
		configureQuery      func(*mock.MockQuery)
		expectErr           bool
		expectedNameServers rootDomainsMap
	}{
		{
			name:        "new name server",
			dnsEndpoint: testDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Create(rootDomain, dnsName, sets.NewString("test-value-1", "test-value-2", "test-value-3")).Return(nil)
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("test-value-1", "test-value-2", "test-value-3"),
					},
				},
			},
		},
		{
			name:        "up-to-date name server",
			dnsEndpoint: testDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("test-value-1", "test-value-2", "test-value-3"),
					},
				},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("test-value-1", "test-value-2", "test-value-3"),
					},
				},
			},
		},
		{
			name:        "out-of-date name server",
			dnsEndpoint: testDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("old-value"),
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Create(rootDomain, dnsName, sets.NewString("test-value-1", "test-value-2", "test-value-3")).Return(nil)
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("test-value-1", "test-value-2", "test-value-3"),
					},
				},
			},
		},
		{
			name:        "delete name server",
			dnsEndpoint: testDeletedDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{
					dnsName: endpointState{
						objectKey: objectKey,
						nsValues:  sets.NewString("test-value-1", "test-value-2", "test-value-3"),
					},
				},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Delete(rootDomain, dnsName, sets.NewString("test-value-1", "test-value-2", "test-value-3")).Return(nil)
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name:        "delete untracked name server",
			dnsEndpoint: testDeletedDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Delete(rootDomain, dnsName, nil).Return(nil)
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name:        "create error",
			dnsEndpoint: testDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Create(rootDomain, dnsName, sets.NewString("test-value-1", "test-value-2", "test-value-3")).
					Return(errors.New("create error"))
			},
			expectErr: true,
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name:        "delete error",
			dnsEndpoint: testDeletedDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			configureQuery: func(mockQuery *mock.MockQuery) {
				mockQuery.EXPECT().Delete(rootDomain, dnsName, nil).
					Return(errors.New("delete error"))
			},
			expectErr: true,
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name:        "name servers not yet scraped",
			dnsEndpoint: testDNSEndpoint(),
			nameServers: rootDomainsMap{
				rootDomain: nil,
			},
			expectErr: true,
			expectedNameServers: rootDomainsMap{
				rootDomain: nil,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			logger := log.WithField("controller", controllerName)
			fakeClient := fake.NewFakeClient(tc.dnsEndpoint)
			mockQuery := mock.NewMockQuery(mockCtrl)
			if tc.configureQuery != nil {
				tc.configureQuery(mockQuery)
			}
			rootDomains := make([]string, len(tc.nameServers))
			for rootDomain := range tc.nameServers {
				rootDomains = append(rootDomains, rootDomain)
			}
			scraper := newNameServerScraper(logger, mockQuery, rootDomains, nil)
			scraper.nameServers = tc.nameServers
			cut := &ReconcileDNSEndpoint{
				Client:            fakeClient,
				scheme:            scheme.Scheme,
				logger:            logger,
				nameServerScraper: scraper,
				nameServerQuery:   mockQuery,
			}
			result, err := cut.Reconcile(reconcile.Request{NamespacedName: objectKey})
			if tc.expectErr {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}
			assert.Equal(t, reconcile.Result{}, result, "unexpected reconcile result")
			assert.Equal(t, tc.expectedNameServers, scraper.nameServers, "unexpected name servers in scraper")
		})
	}
}

func testDNSEndpoint() *hivev1.DNSEndpoint {
	return &hivev1.DNSEndpoint{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  testNamespace,
			Name:       testName,
			Finalizers: []string{hivev1.FinalizerDNSEndpoint},
		},
		Spec: hivev1.DNSEndpointSpec{
			Endpoints: []*hivev1.Endpoint{{
				DNSName:    dnsName,
				RecordType: "NS",
				Targets: []string{
					"test-value-1",
					"test-value-2",
					"test-value-3",
				},
			}},
		},
	}
}

func testDeletedDNSEndpoint() *hivev1.DNSEndpoint {
	e := testDNSEndpoint()
	now := metav1.Now()
	e.DeletionTimestamp = &now
	return e
}

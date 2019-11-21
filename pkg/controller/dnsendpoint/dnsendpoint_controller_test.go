package dnsendpoint

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
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
		name                     string
		dnsZone                  *hivev1.DNSZone
		nameServers              rootDomainsMap
		configureQuery           func(*mock.MockQuery)
		expectErr                bool
		expectedNameServers      rootDomainsMap
		expectedCreatedCondition bool
	}{
		{
			name:    "new name server",
			dnsZone: testDNSZone(),
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
			expectedCreatedCondition: true,
		},
		{
			name:    "up-to-date name server",
			dnsZone: testDNSZone(),
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
			expectedCreatedCondition: true,
		},
		{
			name:    "out-of-date name server",
			dnsZone: testDNSZone(),
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
			expectedCreatedCondition: true,
		},
		{
			name:    "delete name server",
			dnsZone: testDeletedDNSZone(),
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
			name:    "delete untracked name server",
			dnsZone: testDeletedDNSZone(),
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
			name:    "create error",
			dnsZone: testDNSZone(),
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
			name:    "delete error",
			dnsZone: testDeletedDNSZone(),
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
			name:    "name servers not yet scraped",
			dnsZone: testDNSZone(),
			nameServers: rootDomainsMap{
				rootDomain: nil,
			},
			expectErr: true,
			expectedNameServers: rootDomainsMap{
				rootDomain: nil,
			},
		},
		{
			name: "no link to parent domain",
			dnsZone: func() *hivev1.DNSZone {
				z := testDNSZone()
				z.Spec.LinkToParentDomain = false
				return z
			}(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name: "deleted with no finalizer",
			dnsZone: func() *hivev1.DNSZone {
				z := testDeletedDNSZone()
				z.Finalizers = nil
				return z
			}(),
			nameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
			expectedNameServers: rootDomainsMap{
				rootDomain: nameServersMap{},
			},
		},
		{
			name: "emptied name server",
			dnsZone: func() *hivev1.DNSZone {
				z := testDNSZone()
				z.Status.NameServers = nil
				z.Status.Conditions = []hivev1.DNSZoneCondition{{
					Type:   hivev1.ParentLinkCreatedCondition,
					Status: corev1.ConditionTrue,
				}}
				return z
			}(),
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			logger := log.WithField("controller", controllerName)
			fakeClient := fake.NewFakeClient(tc.dnsZone)
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
			dnsZone := &hivev1.DNSZone{}
			if err := fakeClient.Get(context.Background(), objectKey, dnsZone); assert.NoError(t, err, "unexpected error getting DNSZone") {
				cond := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, hivev1.ParentLinkCreatedCondition)
				if tc.expectedCreatedCondition {
					if assert.NotNil(t, cond, "expected to find ParentLinkCreated condition") {
						assert.Equal(t, corev1.ConditionTrue, cond.Status, "expected ParentLinkCreated condition to be True")
					}
				} else {
					assert.True(t, cond == nil || cond.Status == corev1.ConditionFalse, "expected ParentLinkCreated condition to not be True")
				}
			}
		})
	}
}

func testDNSZone() *hivev1.DNSZone {
	return &hivev1.DNSZone{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  testNamespace,
			Name:       testName,
			Finalizers: []string{hivev1.FinalizerDNSEndpoint},
		},
		Spec: hivev1.DNSZoneSpec{
			Zone:               dnsName,
			LinkToParentDomain: true,
		},
		Status: hivev1.DNSZoneStatus{
			NameServers: []string{
				"test-value-1",
				"test-value-2",
				"test-value-3",
			},
		},
	}
}

func testDeletedDNSZone() *hivev1.DNSZone {
	e := testDNSZone()
	now := metav1.Now()
	e.DeletionTimestamp = &now
	return e
}

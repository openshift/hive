package dnsendpoint

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	testNamespace    = "test-namespace"
	testName         = "test-name"
	rootDomain       = "domain.com"
	dnsName          = "test.domain.com"
	cloudCredsSecret = "aws-dns-creds"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

type conditionExpectations struct {
	conditionType hivev1.DNSZoneConditionType
	status        corev1.ConditionStatus
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
		expectedConditions       []conditionExpectations
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionTrue,
				},
			},
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionTrue,
				},
			},
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionTrue,
				},
			},
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionFalse,
				},
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionFalse,
				},
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
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.ParentLinkCreatedCondition,
					status:        corev1.ConditionFalse,
				},
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
		{
			name:    "missing domain client condition",
			dnsZone: testDNSZone(),
			nameServers: rootDomainsMap{
				"notdomain.com": nameServersMap{},
			},
			expectedNameServers: rootDomainsMap{
				"notdomain.com": nameServersMap{},
			},
			expectedConditions: []conditionExpectations{
				{
					conditionType: hivev1.DomainNotManaged,
					status:        corev1.ConditionTrue,
				},
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
				Client: fakeClient,
				scheme: scheme.Scheme,
				logger: logger,
				nameServerTools: []nameServerTool{
					{
						scraper:     scraper,
						queryClient: mockQuery,
					},
				},
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
				validateConditions(t, dnsZone, tc.expectedConditions)
			}
		})
	}
}

func validateConditions(t *testing.T, dnsZone *hivev1.DNSZone, conditions []conditionExpectations) {
	for _, expectedCondition := range conditions {
		cond := controllerutils.FindDNSZoneCondition(dnsZone.Status.Conditions, expectedCondition.conditionType)
		if expectedCondition.status == corev1.ConditionFalse {
			assert.True(t, cond == nil || cond.Status == corev1.ConditionFalse, "expected condition %v to be missing or not be true", expectedCondition.conditionType)
		} else {
			if assert.NotNil(t, cond, "expected to find condition %v", expectedCondition.conditionType) {
				assert.Equal(t, expectedCondition.status, cond.Status, "unexpected status for condition %v", cond.Type)
			}
		}
	}
}

type fakeManager struct {
	watchedDomains map[string]bool
}

func (fm *fakeManager) Add(mgr manager.Runnable) error {
	// not implemented

	scraper, ok := mgr.(*nameServerScraper)
	if ok {
		// record which domains are being watched/scraped
		for domainKey := range scraper.nameServers {
			fm.watchedDomains[domainKey] = true
		}
	}
	return nil
}
func (*fakeManager) Elected() <-chan struct{} {
	panic("not implemented")
}
func (*fakeManager) SetFields(interface{}) error {
	panic("not implemented")
}
func (*fakeManager) AddMetricsExtraHandler(path string, handler http.Handler) error {
	panic("not implemented")
}
func (*fakeManager) AddHealthzCheck(name string, check healthz.Checker) error {
	panic("not implemented")
}
func (*fakeManager) AddReadyzCheck(name string, check healthz.Checker) error {
	panic("not implemented")
}
func (*fakeManager) Start(<-chan struct{}) error {
	panic("not implemented")
}
func (*fakeManager) GetConfig() *rest.Config {
	panic("not implemented")
}
func (*fakeManager) GetScheme() *runtime.Scheme {
	// not implemented
	return nil
}
func (*fakeManager) GetClient() client.Client {
	panic("not implemented")
}
func (*fakeManager) GetFieldIndexer() client.FieldIndexer {
	panic("not implemented")
}
func (*fakeManager) GetCache() cache.Cache {
	panic("not implemented")
}
func (*fakeManager) GetEventRecorderFor(string) record.EventRecorder {
	panic("not implemented")
}
func (*fakeManager) GetRESTMapper() meta.RESTMapper {
	panic("not implemented")
}
func (*fakeManager) GetAPIReader() client.Reader {
	panic("not implemented")
}
func (*fakeManager) GetWebhookServer() *webhook.Server {
	panic("not implemented")
}

func TestMultiCloudDNSSetup(t *testing.T) {

	cases := []struct {
		name           string
		managedDomains []hivev1.ManageDNSConfig
		badDomainsFile bool
		expectedErr    bool
	}{
		{
			name: "single managed domain",
			managedDomains: []hivev1.ManageDNSConfig{
				testManagedDomain(),
			},
		},
		{
			name: "two managed domains same cloud",
			managedDomains: []hivev1.ManageDNSConfig{
				testManagedDomain(),
				func() hivev1.ManageDNSConfig {
					md := testManagedDomain()
					md.Domains = []string{
						"extra.domain.com",
					}
					return md
				}(),
			},
		},
		{
			name: "two managed domains different clouds",
			managedDomains: []hivev1.ManageDNSConfig{
				testManagedDomain(),
				func() hivev1.ManageDNSConfig {
					md := testManagedDomain()
					md.AWS = nil
					md.GCP = &hivev1.ManageDNSGCPConfig{
						CredentialsSecretRef: corev1.LocalObjectReference{
							Name: "gcp-dns-creds",
						},
					}
					md.Domains = []string{
						"gcp.domain.com",
					}
					return md
				}(),
			},
		},
		{
			name: "managed domain entry with multiple domains listed",
			managedDomains: []hivev1.ManageDNSConfig{
				func() hivev1.ManageDNSConfig {
					md := testManagedDomain()
					md.Domains = []string{
						"first.domain.com",
						"second.domain.com",
					}
					return md
				}(),
			},
		},
		{
			name:           "no manged domain entries",
			managedDomains: []hivev1.ManageDNSConfig{},
		},
		{
			name:           "badly formatted domains file",
			badDomainsFile: true,
			expectedErr:    true,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Set up managed domains file/environment
			tempFile, err := ioutil.TempFile("", "")
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			defer os.Remove(tempFile.Name())

			domainsJSON, err := json.Marshal(test.managedDomains)
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}

			if _, err := tempFile.Write(domainsJSON); err != nil {
				t.Fatalf("unexpected: %v", err)
			}

			if test.badDomainsFile {
				if _, err := tempFile.Write([]byte("some non JSON here")); err != nil {
					t.Fatalf("unexpected: %v", err)
				}
			}

			if err := tempFile.Close(); err != nil {
				t.Fatalf("unexpected: %v", err)
			}

			if err := os.Setenv(constants.ManagedDomainsFileEnvVar, tempFile.Name()); err != nil {
				t.Fatalf("unexpected: %v", err)
			}

			// Run/set up reconciler
			fakeClient := fake.NewFakeClient()
			fakeMgr := &fakeManager{
				watchedDomains: map[string]bool{},
			}

			reconciler, _, err := newReconciler(fakeMgr, fakeClient)

			// Assert
			if test.expectedErr {
				assert.Error(t, err)
			} else {
				for _, md := range test.managedDomains {
					for _, domain := range md.Domains {
						found := false
						for _, nstool := range reconciler.nameServerTools {
							rootDomain, _ := nstool.scraper.GetEndpoint(domain)
							if rootDomain != "" {
								found = true
								break
							}
						}
						assert.True(t, found, "failed to find scraper for domain %s", domain)
						assert.True(t, fakeMgr.watchedDomains[domain], "failed to record domain %s as being watched", domain)
					}
				}
			}
		})
	}
}

func testManagedDomain() hivev1.ManageDNSConfig {
	return hivev1.ManageDNSConfig{
		Domains: []string{
			rootDomain,
		},
		AWS: &hivev1.ManageDNSAWSConfig{
			CredentialsSecretRef: corev1.LocalObjectReference{
				Name: cloudCredsSecret,
			},
		},
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

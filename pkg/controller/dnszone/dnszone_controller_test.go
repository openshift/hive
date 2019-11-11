package dnszone

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/awsclient/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
)

// TestReconcileDNSProvider tests that ReconcileDNSProvider reacts properly under different reconciliation states.
func TestReconcileDNSProvider(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name                string
		dnsZone             *hivev1.DNSZone
		dnsEndpoint         *hivev1.DNSEndpoint
		setupAWSMock        func(*mock.MockClientMockRecorder)
		validateZone        func(*testing.T, *hivev1.DNSZone)
		validateDNSEndpoint func(*testing.T, *hivev1.DNSEndpoint)
		errorExpected       bool
		soaLookupResult     bool
	}{
		{
			name:    "DNSZone without finalizer",
			dnsZone: validDNSZoneWithoutFinalizer(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithoutFinalizer())
				mockExistingTags(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.True(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Create Hosted Zone, No ID Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockCreateZone(expect)
				mockNoExistingTags(expect)
				mockSyncTags(expect)
				mockGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.NotNil(t, zone.Status.AWS)
				assert.NotNil(t, zone.Status.AWS.ZoneID)
				assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone, No ID Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithoutID())
				mockExistingTags(expect)
				mockGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.NotNil(t, zone.Status.AWS)
				assert.NotNil(t, zone.Status.AWS.ZoneID)
				assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone, No ID Set, No Tags Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockCreateZoneDuplicateFailure(expect)
				mockListZonesByNameFound(expect, validDNSZoneWithoutID())
				mockNoExistingTags(expect)
				mockSyncTags(expect)
				mockGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.NotNil(t, zone.Status.AWS)
				assert.NotNil(t, zone.Status.AWS.ZoneID)
				assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Existing zone, sync tags",
			dnsZone: validDNSZoneWithAdditionalTags(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingTags(expect)
				mockSyncTags(expect)
				mockGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.Equal(t, zone.Status.LastSyncGeneration, int64(6))
			},
		},
		{
			name:    "Delete hosted zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingTags(expect)
				mockDeleteZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Delete non-existent hosted zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneDoesntExist(expect, validDNSZoneBeingDeleted())
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Existing zone, link to parent, create DNSEndpoint",
			dnsZone: validDNSZoneWithLinkToParent(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingTags(expect)
				mockGetNSRecord(expect)
			},
			validateDNSEndpoint: func(t *testing.T, endpoint *hivev1.DNSEndpoint) {
				assert.NotNil(t, endpoint, "endpoint record should exist")
			},
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validDNSZoneWithLinkToParent(),
			dnsEndpoint:     validDNSEndpoint(),
			soaLookupResult: true,
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingTags(expect)
				mockGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				condition := controllerutils.FindDNSZoneCondition(zone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
				assert.NotNil(t, condition, "zone available condition should be set on dnszone")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)

			zr, _ := NewAWSActuator(
				log.WithField("controller", controllerName),
				validAWSSecret(),
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)

			r := ReconcileDNSZone{
				Client: mocks.fakeKubeClient,
				logger: zr.logger,
				scheme: scheme.Scheme,
			}

			r.soaLookup = func(string, log.FieldLogger) (bool, error) {
				return tc.soaLookupResult, nil
			}

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			defer mocks.mockCtrl.Finish()

			setFakeDNSZoneInKube(mocks, tc.dnsZone)
			if tc.dnsEndpoint != nil {
				setFakeDNSEndpointInKube(mocks, tc.dnsEndpoint)
			}

			if tc.setupAWSMock != nil {
				tc.setupAWSMock(mocks.mockAWSClient.EXPECT())
			}

			// Act
			_, err := r.reconcileDNSProvider(zr, tc.dnsZone)

			// Assert
			if tc.errorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Validate
			zone := &hivev1.DNSZone{}
			err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: tc.dnsZone.Namespace, Name: tc.dnsZone.Name}, zone)
			if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.validateZone != nil {
				tc.validateZone(t, zone)
			}
			if tc.validateDNSEndpoint != nil {
				endpoint := &hivev1.DNSEndpoint{}
				err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: tc.dnsZone.Namespace, Name: tc.dnsZone.Name + "-ns"}, endpoint)
				if err != nil {
					endpoint = nil
				}
				tc.validateDNSEndpoint(t, endpoint)
			}
		})
	}
}

package dnszone

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	awsmock "github.com/openshift/hive/pkg/awsclient/mock"
	azuremock "github.com/openshift/hive/pkg/azureclient/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpmock "github.com/openshift/hive/pkg/gcpclient/mock"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

// TestReconcileDNSProviderForAWS tests that ReconcileDNSProvider reacts properly under different reconciliation states on AWS.
func TestReconcileDNSProviderForAWS(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name              string
		dnsZone           *hivev1.DNSZone
		setupAWSMock      func(*awsmock.MockClientMockRecorder)
		expectZoneDeleted bool
		validateZone      func(*testing.T, *hivev1.DNSZone)
		errorExpected     bool
		soaLookupResult   bool
	}{
		{
			name:    "DNSZone without finalizer",
			dnsZone: validDNSZoneWithoutFinalizer(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithoutFinalizer())
				mockExistingAWSTags(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.True(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Create Hosted Zone, No ID Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockCreateAWSZone(expect)
				mockNoExistingAWSTags(expect)
				mockSyncAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				if assert.NotNil(t, zone.Status.AWS) {
					if assert.NotNil(t, zone.Status.AWS.ZoneID) {
						assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
					}
				}
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone, No ID Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithoutID())
				mockExistingAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				if assert.NotNil(t, zone.Status.AWS) {
					if assert.NotNil(t, zone.Status.AWS.ZoneID) {
						assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
					}
				}
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone, No ID Set, No Tags Set",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockCreateAWSZoneDuplicateFailure(expect)
				mockListAWSZonesByNameFound(expect, validDNSZoneWithoutID())
				mockNoExistingAWSTags(expect)
				mockSyncAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				if assert.NotNil(t, zone.Status.AWS) {
					if assert.NotNil(t, zone.Status.AWS.ZoneID) {
						assert.Equal(t, *zone.Status.AWS.ZoneID, "1234")
					}
				}
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Existing zone, sync tags",
			dnsZone: validDNSZoneWithAdditionalTags(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockSyncAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.Equal(t, zone.Status.LastSyncGeneration, int64(6))
			},
		},
		{
			name:    "Delete hosted zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockDeleteAWSZone(expect)
			},
			expectZoneDeleted: true,
		},
		{
			name:    "Delete non-existent hosted zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneDoesntExist(expect, validDNSZoneBeingDeleted())
			},
			expectZoneDeleted: true,
		},
		{
			name:    "Delete zone with PreserveOnDelete",
			dnsZone: validDNSZoneBeingDeletedWithPreserve(),
			// No AWS calls expected in this case, our creds may be bad:
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
			},
			expectZoneDeleted: true,
		},
		{
			name: "Delete DNSZone without status",
			dnsZone: func() *hivev1.DNSZone {
				dz := validDNSZoneBeingDeleted()
				dz.Status.AWS = nil
				return dz
			}(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockGetResourcePages(expect)
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockDeleteAWSZone(expect)
			},
			expectZoneDeleted: true,
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				condition := controllerutils.FindCondition(zone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
				assert.NotNil(t, condition, "zone available condition should be set on dnszone")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, tc.dnsZone)

			zr, _ := NewAWSActuator(
				log.WithField("controller", ControllerName),
				mocks.fakeKubeClient,
				awsclient.CredentialsSource{},
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)

			r := ReconcileDNSZone{
				Client: mocks.fakeKubeClient,
				logger: zr.logger,
			}

			r.soaLookup = func(string, log.FieldLogger) (bool, error) {
				return tc.soaLookupResult, nil
			}

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			if tc.setupAWSMock != nil {
				tc.setupAWSMock(mocks.mockAWSClient.EXPECT())
			}

			// Act
			_, err := r.reconcileDNSProvider(zr, tc.dnsZone, zr.logger)

			// Assert
			if tc.errorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Validate
			zone := &hivev1.DNSZone{}
			err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: tc.dnsZone.Namespace, Name: tc.dnsZone.Name}, zone)
			if tc.expectZoneDeleted {
				assert.True(t, apierrors.IsNotFound(err), "expected DNSZone to be deleted")
				// Remainder of the test checks the zone
				return
			} else if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.validateZone != nil {
				tc.validateZone(t, zone)
			}
		})
	}
}

// TestReconcileDNSProviderForGCP tests that ReconcileDNSProvider reacts properly under different reconciliation states on GCP.
func TestReconcileDNSProviderForGCP(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name              string
		dnsZone           *hivev1.DNSZone
		setupGCPMock      func(*gcpmock.MockClientMockRecorder)
		expectZoneDeleted bool
		validateZone      func(*testing.T, *hivev1.DNSZone)
		errorExpected     bool
		soaLookupResult   bool
	}{
		{
			name:    "DNSZone without finalizer",
			dnsZone: validDNSZoneWithoutFinalizer(),
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.True(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Create Managed Zone, No ZoneName Set",
			dnsZone: validDNSZoneWithoutID(),
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneDoesntExist(expect)
				mockCreateGCPZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.NotNil(t, zone.Status.GCP)
				assert.NotNil(t, zone.Status.GCP.ZoneName)
				assert.Equal(t, *zone.Status.GCP.ZoneName, "hive-blah-example-com")
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone, No ZoneName Set",
			dnsZone: validDNSZoneWithoutID(),
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.NotNil(t, zone.Status.GCP)
				assert.NotNil(t, zone.Status.GCP.ZoneName)
				assert.Equal(t, *zone.Status.GCP.ZoneName, "hive-blah-example-com")
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name: "Delete managed zone",
			dnsZone: testdnszone.BasicBuilder().
				Options(
					testdnszone.WithGCPPlatform("testDNSZone"),
				).
				GenericOptions(
					testgeneric.WithNamespace("testNamespace"),
					testgeneric.WithName("testDNSZone"),
					testgeneric.Deleted(),
					testgeneric.WithFinalizer("test-finalizer"),
				).
				Build(),
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneExists(expect)
				mockDeleteGCPZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Delete non-existent managed zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneDoesntExist(expect)
			},
			expectZoneDeleted: true,
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				condition := controllerutils.FindCondition(zone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
				assert.NotNil(t, condition, "zone available condition should be set on dnszone")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, tc.dnsZone)

			zr, _ := NewGCPActuator(
				log.WithField("controller", ControllerName),
				validGCPSecret(),
				tc.dnsZone,
				fakeGCPClientBuilder(mocks.mockGCPClient),
			)

			r := ReconcileDNSZone{
				Client: mocks.fakeKubeClient,
				logger: zr.logger,
			}

			r.soaLookup = func(string, log.FieldLogger) (bool, error) {
				return tc.soaLookupResult, nil
			}

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			if tc.setupGCPMock != nil {
				tc.setupGCPMock(mocks.mockGCPClient.EXPECT())
			}

			// Act
			_, err := r.reconcileDNSProvider(zr, tc.dnsZone, zr.logger)

			// Assert
			if tc.errorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Validate
			zone := &hivev1.DNSZone{}
			err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: tc.dnsZone.Namespace, Name: tc.dnsZone.Name}, zone)
			if tc.expectZoneDeleted {
				assert.True(t, apierrors.IsNotFound(err), "expected DNSZone to be deleted")
				// Remainder of the test uses zone
				return
			} else if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.validateZone != nil {
				tc.validateZone(t, zone)
			}
		})
	}
}

// TestReconcileDNSProviderForAzure tests that ReconcileDNSProvider reacts properly under different reconciliation states on Azure.
func TestReconcileDNSProviderForAzure(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name              string
		dnsZone           *hivev1.DNSZone
		setupAzureMock    func(*gomock.Controller, *azuremock.MockClientMockRecorder)
		expectZoneDeleted bool
		validateZone      func(*testing.T, *hivev1.DNSZone)
		errorExpected     bool
		soaLookupResult   bool
	}{
		{
			name:    "DNSZone without finalizer",
			dnsZone: validAzureDNSZoneWithoutFinalizer(),
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.True(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Create Managed Zone",
			dnsZone: validAzureDNSZone(),
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneDoesntExist(expect)
				mockCreateAzureZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Adopt existing zone",
			dnsZone: validAzureDNSZone(),
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.Equal(t, zone.Status.NameServers, []string{"ns1.example.com", "ns2.example.com"}, "nameservers must be set in status")
			},
		},
		{
			name:    "Delete managed zone",
			dnsZone: validAzureDNSZoneBeingDeleted(),
			setupAzureMock: func(mockCtrl *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneExists(expect)
				mockDeleteAzureZone(mockCtrl, expect)
			},
			expectZoneDeleted: true,
		},
		{
			name:    "Delete non-existent managed zone",
			dnsZone: validAzureDNSZoneBeingDeleted(),
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneDoesntExist(expect)
			},
			expectZoneDeleted: true,
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validAzureDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneExists(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				condition := controllerutils.FindCondition(zone.Status.Conditions, hivev1.ZoneAvailableDNSZoneCondition)
				assert.NotNil(t, condition, "zone available condition should be set on dnszone")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, tc.dnsZone)

			zr, _ := NewAzureActuator(
				log.WithField("controller", ControllerName),
				validAzureSecret(),
				tc.dnsZone,
				fakeAzureClientBuilder(mocks.mockAzureClient),
			)

			r := ReconcileDNSZone{
				Client: mocks.fakeKubeClient,
				logger: zr.logger,
			}

			r.soaLookup = func(string, log.FieldLogger) (bool, error) {
				return tc.soaLookupResult, nil
			}

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			if tc.setupAzureMock != nil {
				tc.setupAzureMock(mocks.mockCtrl, mocks.mockAzureClient.EXPECT())
			}

			// Act
			_, err := r.reconcileDNSProvider(zr, tc.dnsZone, zr.logger)

			// Assert
			if tc.errorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Validate
			zone := &hivev1.DNSZone{}
			err = mocks.fakeKubeClient.Get(context.TODO(), types.NamespacedName{Namespace: tc.dnsZone.Namespace, Name: tc.dnsZone.Name}, zone)
			if tc.expectZoneDeleted {
				assert.True(t, apierrors.IsNotFound(err), "expected DNSZone to be deleted")
				// Remainder of the test uses zone
				return
			} else if err != nil {
				t.Fatalf("unexpected: %v", err)
			}
			if tc.validateZone != nil {
				tc.validateZone(t, zone)
			}
		})
	}
}

// TestReconcileDNSProviderForAWSWithConditions tests that expected conditions are set after calling ReconcileDNSProvider for AWS
func TestReconcileDNSProviderForAWSWithConditions(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name               string
		dnsZone            *hivev1.DNSZone
		setupAWSMock       func(*awsmock.MockClientMockRecorder)
		errorExpected      bool
		expectDnsCondition bool
		dnsCondition       hivev1.DNSZoneCondition
		actuator           string
		soaLookupResult    bool
	}{
		{
			name:    "Fail to create hosted zone, set generic dns error condition",
			dnsZone: validDNSZoneWithoutID(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockAWSZoneCreationError(expect)
			},
			errorExpected:      true,
			expectDnsCondition: true,
			dnsCondition: hivev1.DNSZoneCondition{
				Type:    hivev1.GenericDNSErrorsCondition,
				Status:  corev1.ConditionTrue,
				Reason:  dnsCloudErrorReason,
				Message: "KMSOptInRequired: error creating hosted zone\ncaused by: error creating hosted zone",
			},
		},
		{
			name: "Clear generic error condition",
			dnsZone: func() *hivev1.DNSZone {
				zone := validDNSZoneWithoutID()
				zone.Status.Conditions = controllerutils.SetDNSZoneCondition(
					zone.Status.Conditions,
					hivev1.GenericDNSErrorsCondition,
					corev1.ConditionTrue,
					"RelocationError",
					"Test error",
					controllerutils.UpdateConditionIfReasonOrMessageChange)
				return zone
			}(),
			setupAWSMock: func(expect *awsmock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithoutID())
				mockExistingAWSTags(expect)
				mockAWSGetNSRecord(expect)
			},
			expectDnsCondition: true,
			dnsCondition: hivev1.DNSZoneCondition{
				Type:    hivev1.GenericDNSErrorsCondition,
				Status:  corev1.ConditionFalse,
				Reason:  dnsNoErrorReason,
				Message: "No errors occurred",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t, tc.dnsZone)

			zr, _ := NewAWSActuator(
				log.WithField("controller", ControllerName),
				mocks.fakeKubeClient,
				awsclient.CredentialsSource{},
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)

			r := ReconcileDNSZone{
				Client: mocks.fakeKubeClient,
				logger: zr.logger,
			}

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			r.soaLookup = func(string, log.FieldLogger) (bool, error) {
				return tc.soaLookupResult, nil
			}

			if tc.setupAWSMock != nil {
				tc.setupAWSMock(mocks.mockAWSClient.EXPECT())
			}

			// Act
			_, err := r.reconcileDNSProvider(zr, tc.dnsZone, zr.logger)
			zr.SetConditionsForError(err)

			// Assert
			if tc.errorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Validate
			if tc.expectDnsCondition {
				cond := controllerutils.FindCondition(zr.dnsZone.Status.Conditions, tc.dnsCondition.Type)
				if assert.NotNil(t, cond, "expected condition not found") {
					assert.Equal(t, tc.dnsCondition.Status, cond.Status, "condition in unexpected status")
					assert.Equal(t, tc.dnsCondition.Reason, cond.Reason, "condition with unexpected reason")
					assert.Equal(t, tc.dnsCondition.Message, cond.Message, "condition with unexpected message")
				}
			}
		})
	}
}

func TestIsErrorUpdateEvent(t *testing.T) {
	tests := []struct {
		name string
		evt  event.UpdateEvent

		exp bool
	}{{
		name: "invalid type",
		evt: event.UpdateEvent{
			ObjectNew: &hivev1.ClusterDeployment{},
		},
		exp: false,
	}, {
		name: "empty conditions",
		evt: event.UpdateEvent{
			ObjectNew: validDNSZone(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: false,
	}, {
		name: "no error condition",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.DomainNotManaged,
					Status: corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: false,
	}, {
		name: "error condition set to Unknown",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionUnknown,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: false,
	}, {
		name: "error condition set to False",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionFalse,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: false,
	}, {
		name: "error condition set to True, old empty",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: true,
	}, {
		name: "error condition set to True, old unknown",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionUnknown,
				})
				return z
			}(),
		},
		exp: true,
	}, {
		name: "error condition set to True, old false",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionFalse,
				})
				return z
			}(),
		},
		exp: true,
	}, {
		name: "error condition set to True, old True, message and reason same",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
		},
		exp: false,
	}, {
		name: "error condition set to True, old True, message change",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message changed",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
		},
		exp: true,
	}, {
		name: "error condition set to True, old True, reason change",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeAnotherReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
		},
		exp: true,
	}, {
		name: "error condition set to True, old True, both message and reason change",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeAnotherReason",
					Message: "some message changed",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:    hivev1.InsufficientCredentialsCondition,
					Reason:  "SomeReason",
					Message: "some message",
					Status:  corev1.ConditionTrue,
				})
				return z
			}(),
		},
		exp: true,
	}, {
		name: "multiple error condition, all set to False,",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionFalse,
				}, hivev1.DNSZoneCondition{
					Type:   hivev1.AuthenticationFailureCondition,
					Status: corev1.ConditionFalse,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: false,
	}, {
		name: "multiple error condition, first set to True, old empty",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionTrue,
				}, hivev1.DNSZoneCondition{
					Type:   hivev1.AuthenticationFailureCondition,
					Status: corev1.ConditionFalse,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: true,
	}, {
		name: "multiple error condition, second set to True, old empty",
		evt: event.UpdateEvent{
			ObjectNew: func() *hivev1.DNSZone {
				z := validDNSZone()
				z.Status.Conditions = append(z.Status.Conditions, hivev1.DNSZoneCondition{
					Type:   hivev1.InsufficientCredentialsCondition,
					Status: corev1.ConditionFalse,
				}, hivev1.DNSZoneCondition{
					Type:   hivev1.AuthenticationFailureCondition,
					Status: corev1.ConditionTrue,
				})
				return z
			}(),
			ObjectOld: &hivev1.DNSZone{},
		},
		exp: true,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsErrorUpdateEvent(tt.evt)
			assert.Equal(t, tt.exp, got)
		})
	}
}

func TestSetConditionsForErrorForAWS(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name             string
		dnsZone          *hivev1.DNSZone
		error            error
		expectConditions []hivev1.DNSZoneCondition
	}{
		{
			name:    "Set InsufficientCredentialsCondition on DNSZone for AccessDeniedException error",
			dnsZone: validDNSZone(),
			error:   testAccessDeniedExceptionError(),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.InsufficientCredentialsCondition,
					Status:  corev1.ConditionTrue,
					Reason:  accessDeniedReason,
					Message: "User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny",
				},
			},
		},
		{
			name:    "Set AuthenticationFailureCondition on DNSZone for UnrecognizedClientException error",
			dnsZone: validDNSZone(),
			error:   testUnrecognizedClientExceptionError(),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.AuthenticationFailureCondition,
					Status:  corev1.ConditionTrue,
					Reason:  authenticationFailedReason,
					Message: "The security token included in the request is invalid.",
				},
			},
		},
		{
			name:    "Set AuthenticationFailureCondition on DNSZone for InvalidSignatureException error",
			dnsZone: validDNSZone(),
			error:   testInvalidSignatureExceptionError(),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.AuthenticationFailureCondition,
					Status:  corev1.ConditionTrue,
					Reason:  authenticationFailedReason,
					Message: "The request signature we calculated does not match the signature you provided. Check your AWS Secret Access Key and signing method. Consult the service documentation for details.",
				},
			},
		},
		{
			name:    "Set APIOptInRequiredCondition on DNSZone for OptInRequired error",
			dnsZone: validDNSZone(),
			error:   testOptInRequiredError(),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.APIOptInRequiredCondition,
					Status:  corev1.ConditionTrue,
					Reason:  apiOptInRequiredReason,
					Message: "The AWS Access Key Id needs a subscription for the service.",
				},
			},
		},
		{
			name:    "Set GenericDNSErrorsCondition on DNSZone",
			dnsZone: validDNSZone(),
			error:   testCloudError(),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.GenericDNSErrorsCondition,
					Status:  corev1.ConditionTrue,
					Reason:  dnsCloudErrorReason,
					Message: "ErrCodeKMSOptInRequired: The AWS Access Key Id needs a subscription for the service, status code: 403\ncaused by: some cloud error",
				},
			},
		},
		{
			name: "Clear conditions when no error",
			dnsZone: func() *hivev1.DNSZone {
				dz := validDNSZone()
				dz.Status.Conditions = []hivev1.DNSZoneCondition{
					{
						Type:    hivev1.InsufficientCredentialsCondition,
						Status:  corev1.ConditionTrue,
						Reason:  accessDeniedReason,
						Message: "User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny",
					},
					{
						Type:    hivev1.GenericDNSErrorsCondition,
						Status:  corev1.ConditionTrue,
						Reason:  dnsCloudErrorReason,
						Message: "ErrCodeKMSOptInRequired: The AWS Access Key Id needs a subscription for the service, status code: 403\ncaused by: some cloud error",
					},
					{
						Type:    hivev1.AuthenticationFailureCondition,
						Status:  corev1.ConditionTrue,
						Reason:  authenticationFailedReason,
						Message: "The security token included in the request is invalid.",
					},
					{
						Type:    hivev1.APIOptInRequiredCondition,
						Status:  corev1.ConditionTrue,
						Reason:  apiOptInRequiredReason,
						Message: "The AWS Access Key Id needs a subscription for the service.",
					},
				}

				return dz
			}(),
		},
		{
			name: "Process generic error",
			dnsZone: func() *hivev1.DNSZone {
				dz := validDNSZone()
				dz.Status.Conditions = []hivev1.DNSZoneCondition{
					{
						Type:    hivev1.InsufficientCredentialsCondition,
						Status:  corev1.ConditionTrue,
						Reason:  accessDeniedReason,
						Message: "User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny",
					},
					{
						Type:    hivev1.AuthenticationFailureCondition,
						Status:  corev1.ConditionTrue,
						Reason:  authenticationFailedReason,
						Message: "The security token included in the request is invalid.",
					},
				}

				return dz
			}(),
			error: fmt.Errorf("Just some generic error here"),
			expectConditions: []hivev1.DNSZoneCondition{
				{
					Type:    hivev1.GenericDNSErrorsCondition,
					Status:  corev1.ConditionTrue,
					Reason:  dnsCloudErrorReason,
					Message: "Just some generic error here",
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)

			zr, _ := NewAWSActuator(
				log.WithField("controller", ControllerName),
				mocks.fakeKubeClient,
				awsclient.CredentialsSource{},
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)

			zr.dnsZone = tc.dnsZone

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.

			// Act
			zr.SetConditionsForError(tc.error)

			// Assert
			if tc.expectConditions != nil {
				assertDNSZoneConditions(t, zr.dnsZone, tc.expectConditions)
			} else {
				// Assuming if you didn't expect a condition, there shouldn't be any set to True.
				for _, cond := range zr.dnsZone.Status.Conditions {
					assert.Equal(t, corev1.ConditionFalse, cond.Status)
				}
			}
		})
	}
}

func assertDNSZoneConditions(t *testing.T, dnsZone *hivev1.DNSZone, expectedConditions []hivev1.DNSZoneCondition) {
	assert.LessOrEqual(t, len(expectedConditions), len(dnsZone.Status.Conditions), "some conditions are not present")
	for _, expectedCond := range expectedConditions {
		condition := controllerutils.FindCondition(dnsZone.Status.Conditions, expectedCond.Type)
		if assert.NotNilf(t, condition, "did not find expected condition type: %v", expectedCond.Type) {
			assert.Equal(t, expectedCond.Status, condition.Status, "condition found with unexpected status")
			assert.Equal(t, expectedCond.Reason, condition.Reason, "condition found with unexpected reason")
		}
	}
}

func testCloudError() error {
	dnsCloudErr := awserr.New("ErrCodeKMSOptInRequired",
		"The AWS Access Key Id needs a subscription for the service\n\tstatus code: 403, request id: 0604c1a4-0a68-4d1a-b8e6-cdcf68176d71",
		fmt.Errorf("some cloud error"))
	return dnsCloudErr
}

func testAccessDeniedExceptionError() error {
	accessDeniedErr := awserr.New("AccessDeniedException",
		"User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny",
		fmt.Errorf("User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny"))
	return accessDeniedErr
}

func testUnrecognizedClientExceptionError() error {
	unrecognizedClientErr := awserr.New("UnrecognizedClientException",
		"The security token included in the request is invalid.",
		fmt.Errorf("The security token included in the request is invalid"))
	return unrecognizedClientErr
}

func testInvalidSignatureExceptionError() error {
	invalidSignatureErr := awserr.New("InvalidSignatureException",
		"The request signature we calculated does not match the signature you provided. Check your AWS Secret Access Key and signing method. Consult the service documentation for details.",
		fmt.Errorf("The request signature we calculated does not match the signature you provided. Check your AWS Secret Access Key and signing method. Consult the service documentation for details"))
	return invalidSignatureErr
}

func testOptInRequiredError() error {
	optInReqErr := awserr.New("OptInRequired",
		"The AWS Access Key Id needs a subscription for the service\n\tstatus code: 403, request id: f5afff49-3e3d-46d7-92e5-5f9992757398",
		fmt.Errorf("The AWS Access Key Id needs a subscription for the service"))
	return optInReqErr
}

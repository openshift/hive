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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient/mock"
	awsmock "github.com/openshift/hive/pkg/awsclient/mock"
	azuremock "github.com/openshift/hive/pkg/azureclient/mock"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	gcpmock "github.com/openshift/hive/pkg/gcpclient/mock"
)

// TestReconcileDNSProviderForAWS tests that ReconcileDNSProvider reacts properly under different reconciliation states on AWS.
func TestReconcileDNSProviderForAWS(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name            string
		dnsZone         *hivev1.DNSZone
		setupAWSMock    func(*awsmock.MockClientMockRecorder)
		validateZone    func(*testing.T, *hivev1.DNSZone)
		errorExpected   bool
		soaLookupResult bool
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
				mockAWSZoneExists(expect, validDNSZoneWithoutID())
				mockExistingAWSTags(expect)
				mockAWSGetNSRecord(expect)
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
				mockAWSZoneDoesntExist(expect, validDNSZoneWithoutID())
				mockCreateAWSZoneDuplicateFailure(expect)
				mockListAWSZonesByNameFound(expect, validDNSZoneWithoutID())
				mockNoExistingAWSTags(expect)
				mockSyncAWSTags(expect)
				mockAWSGetNSRecord(expect)
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
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockDeleteAWSZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Delete non-existent hosted zone",
			dnsZone: validDNSZoneBeingDeleted(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockAWSZoneDoesntExist(expect, validDNSZoneBeingDeleted())
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name: "Delete DNSZone without status",
			dnsZone: func() *hivev1.DNSZone {
				dz := validDNSZoneBeingDeleted()
				dz.Status.AWS = nil
				return dz
			}(),
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockGetResourcePages(expect)
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockDeleteAWSZone(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupAWSMock: func(expect *mock.MockClientMockRecorder) {
				mockAWSZoneExists(expect, validDNSZoneWithAdditionalTags())
				mockExistingAWSTags(expect)
				mockAWSGetNSRecord(expect)
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
				log.WithField("controller", ControllerName),
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
		})
	}
}

// TestReconcileDNSProviderForGCP tests that ReconcileDNSProvider reacts properly under different reconciliation states on GCP.
func TestReconcileDNSProviderForGCP(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name            string
		dnsZone         *hivev1.DNSZone
		setupGCPMock    func(*gcpmock.MockClientMockRecorder)
		validateZone    func(*testing.T, *hivev1.DNSZone)
		errorExpected   bool
		soaLookupResult bool
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
			name:    "Delete managed zone",
			dnsZone: validDNSZoneBeingDeleted(),
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
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupGCPMock: func(expect *gcpmock.MockClientMockRecorder) {
				mockGCPZoneExists(expect)
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

			zr, _ := NewGCPActuator(
				log.WithField("controller", ControllerName),
				validGCPSecret(),
				tc.dnsZone,
				fakeGCPClientBuilder(mocks.mockGCPClient),
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

			if tc.setupGCPMock != nil {
				tc.setupGCPMock(mocks.mockGCPClient.EXPECT())
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
		})
	}
}

// TestReconcileDNSProviderForAzure tests that ReconcileDNSProvider reacts properly under different reconciliation states on Azure.
func TestReconcileDNSProviderForAzure(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name            string
		dnsZone         *hivev1.DNSZone
		setupAzureMock  func(*gomock.Controller, *azuremock.MockClientMockRecorder)
		validateZone    func(*testing.T, *hivev1.DNSZone)
		errorExpected   bool
		soaLookupResult bool
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
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:    "Delete non-existent managed zone",
			dnsZone: validAzureDNSZoneBeingDeleted(),
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneDoesntExist(expect)
			},
			validateZone: func(t *testing.T, zone *hivev1.DNSZone) {
				assert.False(t, controllerutils.HasFinalizer(zone, hivev1.FinalizerDNSZone))
			},
		},
		{
			name:            "Existing zone, link to parent, reachable SOA",
			dnsZone:         validAzureDNSZoneWithLinkToParent(),
			soaLookupResult: true,
			setupAzureMock: func(_ *gomock.Controller, expect *azuremock.MockClientMockRecorder) {
				mockAzureZoneExists(expect)
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

			zr, _ := NewAzureActuator(
				log.WithField("controller", ControllerName),
				validAzureSecret(),
				tc.dnsZone,
				fakeAzureClientBuilder(mocks.mockAzureClient),
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

			if tc.setupAzureMock != nil {
				tc.setupAzureMock(mocks.mockCtrl, mocks.mockAzureClient.EXPECT())
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
		})
	}
}

func TestSetConditionsForErrorForAWS(t *testing.T) {

	log.SetLevel(log.DebugLevel)

	cases := []struct {
		name            string
		dnsZone         *hivev1.DNSZone
		error           error
		expectCondition *hivev1.DNSZoneCondition
	}{
		{
			name:    "Set InsufficientCredentialsCondition on DNSZone for AccessDeniedException error",
			dnsZone: validDNSZone(),
			error:   testAccessDeniedExceptionError(),
			expectCondition: &hivev1.DNSZoneCondition{
				Type:    hivev1.InsufficientCredentialsCondition,
				Status:  corev1.ConditionTrue,
				Reason:  accessDeniedReason,
				Message: "User: arn:aws:iam::0123456789:user/testAdmin is not authorized to perform: tag:GetResources with an explicit deny",
			},
		},
		{
			name:    "Set AuthenticationFailureCondition on DNSZone for UnrecognizedClientException error",
			dnsZone: validDNSZone(),
			error:   testUnrecognizedClientExceptionError(),
			expectCondition: &hivev1.DNSZoneCondition{
				Type:    hivev1.AuthenticationFailureCondition,
				Status:  corev1.ConditionTrue,
				Reason:  authenticationFailedReason,
				Message: "The security token included in the request is invalid.",
			},
		},
		{
			name:    "Set AuthenticationFailureCondition on DNSZone for InvalidSignatureException error",
			dnsZone: validDNSZone(),
			error:   testInvalidSignatureExceptionError(),
			expectCondition: &hivev1.DNSZoneCondition{
				Type:    hivev1.AuthenticationFailureCondition,
				Status:  corev1.ConditionTrue,
				Reason:  authenticationFailedReason,
				Message: "The request signature we calculated does not match the signature you provided. Check your AWS Secret Access Key and signing method. Consult the service documentation for details.",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)

			zr, _ := NewAWSActuator(
				log.WithField("controller", ControllerName),
				validAWSSecret(),
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)

			zr.dnsZone = tc.dnsZone

			// This is necessary for the mocks to report failures like methods not being called an expected number of times.
			defer mocks.mockCtrl.Finish()

			// Act
			zr.SetConditionsForError(tc.error)

			// Assert
			if tc.expectCondition != nil {
				for _, cond := range zr.dnsZone.Status.Conditions {
					assert.Equal(t, cond.Type, tc.expectCondition.Type)
					assert.Equal(t, cond.Status, tc.expectCondition.Status)
					assert.Equal(t, cond.Reason, tc.expectCondition.Reason)
					assert.Equal(t, cond.Message, tc.expectCondition.Message)
				}
			} else {
				// Assuming if you didn't expect a condition, there shouldn't be any.
				assert.Equal(t, 0, len(zr.dnsZone.Status.Conditions))
			}
		})
	}
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

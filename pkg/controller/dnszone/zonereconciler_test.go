package dnszone

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/mock/gomock"
	"github.com/openshift/hive/pkg/awsclient/mock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

func init() {
	apis.AddToScheme(scheme.Scheme)
}

// TestNewZoneReconciler tests that a new ZoneReconciler object can be created.
func TestNewZoneReconciler(t *testing.T) {
	cases := []struct {
		name              string
		expectedErrString string
		dnsZone           *hivev1.DNSZone
	}{
		{
			name:    "Successfully create new zone",
			dnsZone: validDNSZone(),
		},
		{
			name:              "Fail to create new zone because dnsZone not set",
			expectedErrString: "ZoneReconciler requires dnsZone to be set",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)
			expectedZoneReconciler := &ZoneReconciler{
				dnsZone:    tc.dnsZone,
				kubeClient: mocks.fakeKubeClient,
				logger:     log.WithField("controller", controllerName),
				awsClient:  mocks.mockAWSClient,
				scheme:     scheme.Scheme,
			}

			// Act
			zr, err := NewZoneReconciler(
				expectedZoneReconciler.dnsZone,
				expectedZoneReconciler.kubeClient,
				expectedZoneReconciler.logger,
				expectedZoneReconciler.awsClient,
				expectedZoneReconciler.scheme,
			)
			// Function equality cannot be tested by assert.Equal
			// therefore it is set to nil for comparison
			if zr != nil {
				zr.soaLookup = nil
			}

			// Assert
			assertErrorNilOrMessage(t, err, tc.expectedErrString)
			if tc.expectedErrString == "" {
				// Only check this if we're expected to succeed.
				assert.Equal(t, expectedZoneReconciler, zr)
			}
		})
	}
}

// TestReconcile tests that ZoneReconciler.Reconcile reacts properly under different reconciliation states.
func TestReconcile(t *testing.T) {

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

			zr, _ := NewZoneReconciler(
				tc.dnsZone,
				mocks.fakeKubeClient,
				log.WithField("controller", controllerName),
				mocks.mockAWSClient,
				scheme.Scheme,
			)
			zr.soaLookup = func(string, log.FieldLogger) (bool, error) {
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
			_, err := zr.Reconcile()

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

func mockZoneExists(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {

	if zone.Status.AWS == nil || aws.StringValue(zone.Status.AWS.ZoneID) == "" {
		expect.GetResourcesPages(gomock.Any(), gomock.Any()).
			Do(func(input *resourcegroupstaggingapi.GetResourcesInput, f func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) {
				f(&resourcegroupstaggingapi.GetResourcesOutput{
					ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{
						{
							ResourceARN: aws.String("arn:aws:route53:::hostedzone/1234"),
						},
					},
				}, true)
			}).Return(nil).Times(1)
	}
	expect.GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
		HostedZone: &route53.HostedZone{
			Id:   aws.String("1234"),
			Name: aws.String("blah.example.com"),
		},
	}, nil).Times(1)
}

func mockZoneDoesntExist(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	if zone.Status.AWS != nil && aws.StringValue(zone.Status.AWS.ZoneID) != "" {
		expect.GetHostedZone(gomock.Any()).
			Return(nil, awserr.New(route53.ErrCodeNoSuchHostedZone, "doesnt exist", fmt.Errorf("doesnt exist"))).Times(1)
		return
	}
	expect.GetResourcesPages(gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func mockCreateZone(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
		HostedZone: &route53.HostedZone{
			Id:   aws.String("1234"),
			Name: aws.String("blah.example.com"),
		},
	}, nil).Times(1)
}

func mockCreateZoneDuplicateFailure(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(nil, awserr.New(route53.ErrCodeHostedZoneAlreadyExists, "already exists", fmt.Errorf("already exists"))).Times(1)
}

func mockNoExistingTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(gomock.Any()).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags:       []*route53.Tag{},
		},
	}, nil).Times(1)
}

func mockExistingTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(gomock.Any()).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags: []*route53.Tag{
				{
					Key:   aws.String(hiveDNSZoneTag),
					Value: aws.String("ns/dnszoneobject"),
				},
				{
					Key:   aws.String("foo"),
					Value: aws.String("bar"),
				},
			},
		},
	}, nil).Times(1)
}

func mockSyncTags(expect *mock.MockClientMockRecorder) {
	expect.ChangeTagsForResource(gomock.Any()).Return(&route53.ChangeTagsForResourceOutput{}, nil).AnyTimes()
}

func mockGetNSRecord(expect *mock.MockClientMockRecorder) {
	expect.ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []*route53.ResourceRecordSet{
			{
				Type: aws.String("NS"),
				Name: aws.String("blah.example.com."),
				ResourceRecords: []*route53.ResourceRecord{
					{
						Value: aws.String("ns1.example.com"),
					},
					{
						Value: aws.String("ns2.example.com"),
					},
				},
			},
		},
	}, nil)
}

func mockListZonesByNameFound(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	expect.ListHostedZonesByName(gomock.Any()).Return(&route53.ListHostedZonesByNameOutput{
		HostedZones: []*route53.HostedZone{
			{
				Id:              aws.String("1234"),
				Name:            aws.String("blah.example.com"),
				CallerReference: aws.String(string(zone.UID)),
			},
		},
	}, nil).Times(1)
}

func mockDeleteZone(expect *mock.MockClientMockRecorder) {
	expect.DeleteHostedZone(gomock.Any()).Return(nil, nil).Times(1)
}

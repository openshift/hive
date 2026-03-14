package dnszone

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	tagtypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"go.uber.org/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/awsclient/mock"
)

// TestNewAWSActuator tests that a new AWSActuator object can be created.
func TestNewAWSActuator(t *testing.T) {
	cases := []struct {
		name    string
		dnsZone *hivev1.DNSZone
	}{
		{
			name:    "Successfully create new zone",
			dnsZone: validDNSZone(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)
			expectedAWSActuator := &AWSActuator{
				logger:  log.WithField("controller", ControllerName),
				dnsZone: tc.dnsZone,
			}

			// Act
			zr, err := NewAWSActuator(
				expectedAWSActuator.logger,
				nil, awsclient.CredentialsSource{},
				tc.dnsZone,
				fakeAWSClientBuilder(mocks.mockAWSClient),
			)
			expectedAWSActuator.awsClient = zr.awsClient // Function pointers can't be compared reliably. Don't compare.

			// Assert
			assert.Nil(t, err)
			assert.NotNil(t, zr.awsClient)
			assert.Equal(t, expectedAWSActuator, zr)
		})
	}
}

func mockAWSZoneExists(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {

	if zone.Status.AWS == nil || aws.ToString(zone.Status.AWS.ZoneID) == "" {
		expect.GetResourcesPages(gomock.Any(), gomock.Any()).
			Do(func(input *resourcegroupstaggingapi.GetResourcesInput, f func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) {
				f(&resourcegroupstaggingapi.GetResourcesOutput{
					ResourceTagMappingList: []tagtypes.ResourceTagMapping{
						{
							ResourceARN: aws.String("arn:aws:route53:::hostedzone/1234"),
						},
					},
				}, true)
			}).Return(nil).Times(1)
	}
	expect.GetHostedZone(gomock.Any()).Return(&route53.GetHostedZoneOutput{
		HostedZone: &route53types.HostedZone{
			Id:   aws.String("/hostedzone/1234"),
			Name: aws.String("blah.example.com."),
		},
	}, nil).Times(1)
}

func mockAWSZoneDoesntExist(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	if zone.Status.AWS != nil && aws.ToString(zone.Status.AWS.ZoneID) != "" {
		expect.GetHostedZone(gomock.Any()).
			Return(nil, awsclient.NewAPIError("NoSuchHostedZone", "")).Times(1)
		return
	}
	expect.GetResourcesPages(gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func mockAWSZoneCreationError(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).
		Return(nil, awsclient.NewAPIError("KMSOptInRequired", "error creating hosted zone")).Times(1)
}

func mockCreateAWSZone(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
		HostedZone: &route53types.HostedZone{
			Id:   aws.String("/hostedzone/1234"),
			Name: aws.String("blah.example.com."),
		},
	}, nil).Times(1)
}

func mockCreateAWSZoneDuplicateFailure(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(nil, awsclient.NewAPIError("HostedZoneAlreadyExists", "")).Times(1)
}

func mockNoExistingAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(&route53.ListTagsForResourceInput{
		ResourceId:   aws.String("1234"),
		ResourceType: route53types.TagResourceTypeHostedzone,
	}).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53types.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags:       []route53types.Tag{},
		},
	}, nil).Times(1)
}

func mockExistingAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(&route53.ListTagsForResourceInput{
		ResourceId:   aws.String("1234"),
		ResourceType: route53types.TagResourceTypeHostedzone,
	}).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53types.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags: []route53types.Tag{
				{
					Key:   aws.String(hiveDNSZoneAWSTag),
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

type ctfriMatcher struct {
	want *route53.ChangeTagsForResourceInput
}

func newctfriMatcher(input *route53.ChangeTagsForResourceInput) gomock.Matcher {
	return &ctfriMatcher{want: input}
}

func (m *ctfriMatcher) Matches(x any) bool {
	ctfri, ok := x.(*route53.ChangeTagsForResourceInput)
	if !ok {
		return false
	}
	return *ctfri.ResourceId == *m.want.ResourceId && ctfri.ResourceType == m.want.ResourceType
}

func ctfriString(ctfri *route53.ChangeTagsForResourceInput) string {
	rid := "<nil>"
	if ctfri.ResourceId != nil {
		rid = *ctfri.ResourceId
	}
	return fmt.Sprintf("ResourceId: %q, ResourceType: %q", rid, ctfri.ResourceType)
}

func (m *ctfriMatcher) String() string {
	return ctfriString(m.want)
}

func (m *ctfriMatcher) Got(got any) string {
	return ctfriString(got.(*route53.ChangeTagsForResourceInput))
}

func mockSyncAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ChangeTagsForResource(newctfriMatcher(
		&route53.ChangeTagsForResourceInput{
			ResourceId:   aws.String("1234"),
			ResourceType: route53types.TagResourceTypeHostedzone,
		},
	)).Return(&route53.ChangeTagsForResourceOutput{}, nil).AnyTimes()
}

func mockAWSGetNSRecord(expect *mock.MockClientMockRecorder) {
	expect.ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []route53types.ResourceRecordSet{
			{
				Type: route53types.RRTypeNs,
				Name: aws.String("blah.example.com."),
				ResourceRecords: []route53types.ResourceRecord{
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

func mockListAWSZonesByNameFound(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	expect.ListHostedZonesByName(gomock.Any()).Return(&route53.ListHostedZonesByNameOutput{
		HostedZones: []route53types.HostedZone{
			{
				Id:              aws.String("/hostedzone/1234"),
				Name:            aws.String("blah.example.com."),
				CallerReference: aws.String(string(zone.UID)),
			},
		},
	}, nil).Times(1)
}

func mockDeleteAWSZone(expect *mock.MockClientMockRecorder) {
	expect.ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{}, nil).Times(1)
	expect.DeleteHostedZone(gomock.Any()).Return(nil, nil).Times(1)
}

func mockGetResourcePages(expect *mock.MockClientMockRecorder) {
	expect.GetResourcesPages(gomock.Any(), gomock.Any()).Return(nil).Do(func(i *resourcegroupstaggingapi.GetResourcesInput, f func(*resourcegroupstaggingapi.GetResourcesOutput, bool) bool) {
		getResourcesOutput := &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: []tagtypes.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:route53:::hostedzone/Z055920326CHQAW0WSG5N"),
				},
			},
		}
		f(getResourcesOutput, true)
	})
}

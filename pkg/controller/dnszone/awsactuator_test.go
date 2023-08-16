package dnszone

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/kinesis"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/golang/mock/gomock"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/awsclient/mock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
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
			Name: aws.String("blah.example.com."),
		},
	}, nil).Times(1)
}

func mockAWSZoneDoesntExist(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	if zone.Status.AWS != nil && aws.StringValue(zone.Status.AWS.ZoneID) != "" {
		expect.GetHostedZone(gomock.Any()).
			Return(nil, awserr.New(route53.ErrCodeNoSuchHostedZone, "doesnt exist", fmt.Errorf("doesnt exist"))).Times(1)
		return
	}
	expect.GetResourcesPages(gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

func mockAWSZoneCreationError(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).
		Return(nil, awserr.New(kinesis.ErrCodeKMSOptInRequired, "error creating hosted zone", fmt.Errorf("error creating hosted zone"))).Times(1)
}

func mockCreateAWSZone(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(&route53.CreateHostedZoneOutput{
		HostedZone: &route53.HostedZone{
			Id:   aws.String("1234"),
			Name: aws.String("blah.example.com."),
		},
	}, nil).Times(1)
}

func mockCreateAWSZoneDuplicateFailure(expect *mock.MockClientMockRecorder) {
	expect.CreateHostedZone(gomock.Any()).Return(nil, awserr.New(route53.ErrCodeHostedZoneAlreadyExists, "already exists", fmt.Errorf("already exists"))).Times(1)
}

func mockNoExistingAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(gomock.Any()).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags:       []*route53.Tag{},
		},
	}, nil).Times(1)
}

func mockExistingAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ListTagsForResource(gomock.Any()).Return(&route53.ListTagsForResourceOutput{
		ResourceTagSet: &route53.ResourceTagSet{
			ResourceId: aws.String("1234"),
			Tags: []*route53.Tag{
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

func mockSyncAWSTags(expect *mock.MockClientMockRecorder) {
	expect.ChangeTagsForResource(gomock.Any()).Return(&route53.ChangeTagsForResourceOutput{}, nil).AnyTimes()
}

func mockAWSGetNSRecord(expect *mock.MockClientMockRecorder) {
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

func mockListAWSZonesByNameFound(expect *mock.MockClientMockRecorder, zone *hivev1.DNSZone) {
	expect.ListHostedZonesByName(gomock.Any()).Return(&route53.ListHostedZonesByNameOutput{
		HostedZones: []*route53.HostedZone{
			{
				Id:              aws.String("1234"),
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
			ResourceTagMappingList: []*resourcegroupstaggingapi.ResourceTagMapping{
				{
					ResourceARN: aws.String("arn:aws:route53:::hostedzone/Z055920326CHQAW0WSG5N"),
				},
			},
		}
		f(getResourcesOutput, true)
	})
}

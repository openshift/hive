package dnszone

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"go.uber.org/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/azureclient/mock"
)

// TestNewAzureActuator tests that a new AzureActuator object can be created.
func TestNewAzureActuator(t *testing.T) {
	cases := []struct {
		name    string
		dnsZone *hivev1.DNSZone
		secret  *corev1.Secret
	}{
		{
			name:    "Successfully create new zone",
			dnsZone: validAzureDNSZone(),
			secret:  validAzureSecret(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)
			expectedAzureActuator := &AzureActuator{
				logger:  log.WithField("controller", ControllerName),
				dnsZone: tc.dnsZone,
			}

			// Act
			zr, err := NewAzureActuator(
				expectedAzureActuator.logger,
				tc.secret,
				tc.dnsZone,
				fakeAzureClientBuilder(mocks.mockAzureClient),
			)
			expectedAzureActuator.azureClient = zr.azureClient // Function pointers can't be compared reliably. Don't compare.

			// Assert
			assert.Nil(t, err)
			assert.NotNil(t, zr.azureClient)
			assert.Equal(t, expectedAzureActuator, zr)
		})
	}
}

func mockAzureZoneExists(expect *mock.MockClientMockRecorder) {
	expect.GetZone(gomock.Any(), gomock.Any(), gomock.Any()).Return(dns.Zone{
		Name: to.StringPtr("blah.example.com"),
		ZoneProperties: &dns.ZoneProperties{
			NameServers: to.StringSlicePtr([]string{"ns1.example.com", "ns2.example.com"}),
		},
	}, nil).Times(1)
}

func mockAzureZoneDoesntExist(expect *mock.MockClientMockRecorder) {
	expect.GetZone(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(dns.Zone{
			Response: autorest.Response{
				Response: &http.Response{
					StatusCode: http.StatusNotFound,
				},
			},
		}, errors.New("Not found")).Times(1)
}

func mockCreateAzureZone(expect *mock.MockClientMockRecorder) {
	expect.CreateOrUpdateZone(gomock.Any(), gomock.Any(), gomock.Any()).Return(dns.Zone{
		Name: to.StringPtr("blah.example.com"),
		ZoneProperties: &dns.ZoneProperties{
			NameServers: to.StringSlicePtr([]string{"ns1.example.com", "ns2.example.com"}),
		},
	}, nil).Times(1)
}

func mockDeleteAzureZone(mockCtrl *gomock.Controller, expect *mock.MockClientMockRecorder) {
	recordSetPage := mock.NewMockRecordSetPage(mockCtrl)
	recordSetPage.EXPECT().NotDone().Return(false).Times(1)
	expect.ListRecordSetsByZone(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(recordSetPage, nil)
	expect.DeleteZone(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
}

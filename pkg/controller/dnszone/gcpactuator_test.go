package dnszone

import (
	"testing"

	"go.uber.org/mock/gomock"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/gcpclient/mock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	dns "google.golang.org/api/dns/v1"
	"google.golang.org/api/googleapi"
	corev1 "k8s.io/api/core/v1"
)

// TestNewGCPActuator tests that a new GCPActuator object can be created.
func TestNewGCPActuator(t *testing.T) {
	cases := []struct {
		name    string
		dnsZone *hivev1.DNSZone
		secret  *corev1.Secret
	}{
		{
			name:    "Successfully create new zone",
			dnsZone: validDNSZone(),
			secret:  validGCPSecret(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			mocks := setupDefaultMocks(t)
			expectedGCPActuator := &GCPActuator{
				logger:  log.WithField("controller", ControllerName),
				dnsZone: tc.dnsZone,
			}

			// Act
			zr, err := NewGCPActuator(
				expectedGCPActuator.logger,
				tc.secret,
				tc.dnsZone,
				fakeGCPClientBuilder(mocks.mockGCPClient),
			)
			expectedGCPActuator.gcpClient = zr.gcpClient // Function pointers can't be compared reliably. Don't compare.

			// Assert
			assert.Nil(t, err)
			assert.NotNil(t, zr.gcpClient)
			assert.Equal(t, expectedGCPActuator, zr)
		})
	}
}

func mockGCPZoneExists(expect *mock.MockClientMockRecorder) {
	expect.GetManagedZone(gomock.Any()).Return(&dns.ManagedZone{
		DnsName:     "blah.example.com",
		Name:        "hive-blah-example-com",
		NameServers: []string{"ns1.example.com", "ns2.example.com"},
	}, nil).Times(1)
}

func mockGCPZoneDoesntExist(expect *mock.MockClientMockRecorder) {
	expect.GetManagedZone(gomock.Any()).
		Return(nil, &googleapi.Error{Code: 404}).
		Times(1)
}

func mockCreateGCPZone(expect *mock.MockClientMockRecorder) {
	expect.CreateManagedZone(gomock.Any()).Return(&dns.ManagedZone{
		DnsName:     "blah.example.com",
		Name:        "hive-blah-example-com",
		NameServers: []string{"ns1.example.com", "ns2.example.com"},
	}, nil).Times(1)
}

func mockDeleteGCPZone(expect *mock.MockClientMockRecorder) {
	expect.ListResourceRecordSets(gomock.Any(), gomock.Any()).Return(&dns.ResourceRecordSetsListResponse{}, nil)
	expect.DeleteManagedZone(gomock.Any()).Return(nil).Times(1)
}

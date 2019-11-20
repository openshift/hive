package dnszone

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
				logger:  log.WithField("controller", controllerName),
				dnsZone: tc.dnsZone,
			}

			zr, err := NewGCPActuator(
				expectedGCPActuator.logger,
				tc.secret,
				tc.dnsZone,
				fakeGCPClientBuilder(mocks.mockGCPClient),
			)

			// Act
			expectedGCPActuator.gcpClient = zr.gcpClient // Function pointers can't be compared reliably. Don't compare.

			// Assert
			assert.Nil(t, err)
			assert.NotNil(t, zr.gcpClient)
			assert.Equal(t, expectedGCPActuator, zr)
		})
	}
}

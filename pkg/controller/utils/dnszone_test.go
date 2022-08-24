package utils

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

func TestReconcileDNSZoneForRelocation(t *testing.T) {
	const (
		testNamespace   = "test-namespace"
		testDNSZoneName = "test-dnszone"
		testFinalizer   = "test-finalizer"
	)
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	cases := []struct {
		name              string
		dnsZone           *hivev1.DNSZone
		expectResult      bool
		expectError       bool
		expectNoFinalizer bool
	}{
		{
			name: "no relocate annotation",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
		},
		{
			name: "relocate outgoing",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
					testgeneric.WithAnnotation(constants.RelocateAnnotation, "some-relocate/outgoing"),
				).
				Build(),
			expectResult: true,
		},
		{
			name: "relocate complete",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
					testgeneric.WithAnnotation(constants.RelocateAnnotation, "some-relocate/complete"),
				).
				Build(),
			expectResult:      true,
			expectNoFinalizer: true,
		},
		{
			name: "relocate incoming",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
					testgeneric.WithAnnotation(constants.RelocateAnnotation, "some-relocate/incoming"),
				).
				Build(),
			expectResult: true,
		},
		{
			name: "relocate with unknown status",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
					testgeneric.WithAnnotation(constants.RelocateAnnotation, "some-relocate/other-status"),
				).
				Build(),
			expectError: true,
		},
		{
			name: "relocate with malformed annotation value",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithFinalizer(testFinalizer),
					testgeneric.WithAnnotation(constants.RelocateAnnotation, "bad-value"),
				).
				Build(),
			expectError: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := log.WithField("", "")
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tc.dnsZone).Build()
			result, err := ReconcileDNSZoneForRelocation(client, logger, tc.dnsZone, testFinalizer)
			if tc.expectResult {
				assert.NotNil(t, result, "expected result")
			} else {
				assert.Nil(t, result, "expected no result")
			}
			if tc.expectError {
				assert.Error(t, err, "expected error")
			} else {
				assert.NoError(t, err, "expected no error")
			}
			if tc.expectNoFinalizer {
				assert.NotContains(t, tc.dnsZone.Finalizers, testFinalizer, "expected no finalizer")
			} else {
				assert.Contains(t, tc.dnsZone.Finalizers, testFinalizer, "expected finalizer")
			}
		})
	}
}

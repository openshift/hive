package utils

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

func TestReconcileDNSZoneForRelocation(t *testing.T) {
	const (
		testNamespace   = "test-namespace"
		testDNSZoneName = "test-dnszone"
		testCDName      = "test-clusterdeployment"
		testFinalizer   = "test-finalizer"
	)
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	cases := []struct {
		name              string
		dnsZone           *hivev1.DNSZone
		cd                *hivev1.ClusterDeployment
		expectResult      bool
		expectError       bool
		expectNoFinalizer bool
	}{
		{
			name: "no dnszone type",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(),
		},
		{
			name: "other dnszone type",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, "other-type"),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(),
		},
		{
			name: "no clusterdeployment label",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(),
		},
		{
			name: "no clusterdeployment",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, "other-clusterdeployment"),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd:           testcd.FullBuilder(testNamespace, testCDName, scheme).Build(),
			expectResult: true,
		},
		{
			name: "relocating clusterdeployment",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).
				GenericOptions(
					testgeneric.WithAnnotation(constants.RelocatingAnnotation, "relocator"),
				).Build(),
			expectResult: true,
		},
		{
			name: "relocated clusterdeployment",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).
				GenericOptions(
					testgeneric.WithAnnotation(constants.RelocatedAnnotation, "relocator"),
				).Build(),
			expectResult:      true,
			expectNoFinalizer: true,
		},
		{
			name: "relocated clusterdeployment with no finalizer",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).
				GenericOptions(
					testgeneric.WithAnnotation(constants.RelocatedAnnotation, "relocator"),
				).Build(),
			expectResult:      true,
			expectNoFinalizer: true,
		},
		{
			name: "non-relocating clusterdeployment",
			dnsZone: testdnszone.FullBuilder(testNamespace, testDNSZoneName, scheme).
				GenericOptions(
					testgeneric.WithLabel(constants.DNSZoneTypeLabel, constants.DNSZoneTypeChild),
					testgeneric.WithLabel(constants.ClusterDeploymentNameLabel, testCDName),
					testgeneric.WithFinalizer(testFinalizer),
				).
				Build(),
			cd: testcd.FullBuilder(testNamespace, testCDName, scheme).Build(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger := log.WithField("", "")
			client := fake.NewFakeClientWithScheme(scheme, tc.dnsZone, tc.cd)
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

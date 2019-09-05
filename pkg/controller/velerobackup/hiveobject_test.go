package velerobackup

import (
	"fmt"
	"math"
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestNewHiveObject(t *testing.T) {
	tests := []struct {
		name               string
		object             runtime.Object
		logger             log.FieldLogger
		expectedHiveObject *hiveObject
		expectedError      error
	}{
		{
			name:   "Only Valid ClusterDeployment",
			object: testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			expectedHiveObject: &hiveObject{
				object:   testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				checksum: testclusterdeployment.Build(unchangedClusterDeploymentBase()).Annotations[controllerutils.LastBackupAnnotation],
			},
		},
		{
			name:   "Only Valid SyncSet",
			object: testsyncset.Build(unchangedSyncSetBase()),
			expectedHiveObject: &hiveObject{
				object:   testsyncset.Build(unchangedSyncSetBase()),
				checksum: testsyncset.Build(unchangedSyncSetBase()).Annotations[controllerutils.LastBackupAnnotation],
			},
		},
		{
			name:   "Only Valid DNSZone",
			object: testdnszone.Build(unchangedDNSZoneBase()),
			expectedHiveObject: &hiveObject{
				object:   testdnszone.Build(unchangedDNSZoneBase()),
				checksum: testdnszone.Build(unchangedDNSZoneBase()).Annotations[controllerutils.LastBackupAnnotation],
			},
		},
		{
			name:               "Invalid type (not a hive object)",
			object:             &corev1.Pod{},
			expectedHiveObject: nil,
			expectedError:      fmt.Errorf("Unknown Type: *v1.Pod"),
		},
		{
			name:               "Invalid nil object",
			object:             nil,
			expectedHiveObject: nil,
			expectedError:      fmt.Errorf("Unknown Type: <nil>"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange

			// Act
			actualHiveObject, actualError := newHiveObject(test.object, test.logger)

			// Assert
			assert.Equal(t, test.expectedHiveObject, actualHiveObject)
			assert.Equal(t, test.expectedError, actualError)
		})
	}
}

func TestHasChanged(t *testing.T) {
	tests := []struct {
		name            string
		hiveObject      *hiveObject
		expectedChanged bool
	}{
		{
			name:            "Unchanged ClusterDeployment",
			hiveObject:      ro2ho(testclusterdeployment.Build(unchangedClusterDeploymentBase())),
			expectedChanged: false,
		},
		{
			name:            "Changed ClusterDeployment",
			hiveObject:      ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
			expectedChanged: true,
		},
		{
			name:            "Unchanged SyncSet",
			hiveObject:      ro2ho(testsyncset.Build(unchangedSyncSetBase())),
			expectedChanged: false,
		},
		{
			name:            "Changed SyncSet",
			hiveObject:      ro2ho(testsyncset.Build(changedSyncSetBase())),
			expectedChanged: true,
		},
		{
			name:            "Unchanged DNSZone",
			hiveObject:      ro2ho(testdnszone.Build(unchangedDNSZoneBase())),
			expectedChanged: false,
		},
		{
			name:            "Changed DNSZone",
			hiveObject:      ro2ho(testdnszone.Build(changedDNSZoneBase())),
			expectedChanged: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange

			// Act
			actualChanged := test.hiveObject.hasChanged()

			// Assert
			assert.Equal(t, test.expectedChanged, actualChanged)
		})
	}
}

func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name             string
		meta             metav1.ObjectMeta
		spec             interface{}
		expectedChecksum string
	}{
		{
			name:             "Valid ClusterDeployment Checksum",
			meta:             testclusterdeployment.Build(unchangedClusterDeploymentBase()).ObjectMeta,
			spec:             testclusterdeployment.Build(unchangedClusterDeploymentBase()).Spec,
			expectedChecksum: testclusterdeployment.Build(unchangedClusterDeploymentBase()).Annotations[controllerutils.LastBackupAnnotation],
		},
		{
			name:             "Error from Checksum func",
			meta:             metav1.ObjectMeta{},
			spec:             math.Inf(1),
			expectedChecksum: errChecksum,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange

			// Act
			actualChecksum := calculateChecksum(&test.meta, test.spec, defaultLogger())

			// Assert
			assert.Equal(t, test.expectedChecksum, actualChecksum)
		})
	}
}

func TestSetChecksumAnnotation(t *testing.T) {
	// Arrange
	expectedChecksum := "new-checksum"
	ro := &hivev1.ClusterDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{controllerutils.LastBackupAnnotation: "old-checksum"},
		},
	}
	hiveObject := &hiveObject{
		object:   ro,
		checksum: expectedChecksum,
	}

	// Act
	hiveObject.setChecksumAnnotation()
	actualChecksum := ro.Annotations[controllerutils.LastBackupAnnotation]

	// Assert
	assert.Equal(t, expectedChecksum, actualChecksum, "unexpected checksum")
}

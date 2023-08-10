package velerobackup

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testassert "github.com/openshift/hive/pkg/test/assert"
	testcheckpoint "github.com/openshift/hive/pkg/test/checkpoint"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	"github.com/openshift/hive/pkg/util/scheme"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	namespace      = "notarealns"
	checkpointName = "hive"
)

var (
	twoMinutesAgo     = metav1.NewTime(time.Now().Add(-twoMinuteDuration))
	fiveHoursAgo      = metav1.NewTime(time.Now().Add(-5 * time.Hour))
	twoMinuteDuration = 2 * time.Minute

	statusErr = &kerrors.StatusError{
		ErrStatus: metav1.Status{},
	}

	emptyRuntimeObjectSlice = []runtime.Object{}
)

func calculateRuntimeObjectsChecksum(objects []runtime.Object) string {
	r := fakeClientReconcileBackup(emptyRuntimeObjectSlice)
	return r.calculateObjectsChecksumWithoutStatus(r.logger, objects...)
}

func ignoreUncomparedFields(expected, actual []runtime.Object) (hivev1.BackupReference, metav1.Time) {
	// We need this for the comparisons to work.
	if len(expected) > 0 {
		expectedCheckpoint := expected[len(expected)-1].(*hivev1.Checkpoint)
		actualCheckpoint := actual[len(actual)-1].(*hivev1.Checkpoint)

		// These are fields that we do not want to compare.
		expectedCheckpoint.Spec.LastBackupTime = actualCheckpoint.Spec.LastBackupTime
		expectedCheckpoint.Spec.LastBackupRef = actualCheckpoint.Spec.LastBackupRef

		return actualCheckpoint.Spec.LastBackupRef, actualCheckpoint.Spec.LastBackupTime
	}

	return hivev1.BackupReference{Namespace: namespace}, metav1.Now()
}

func calculateErrorChecksum() string {
	checksums := []string{errChecksum}

	combinedChecksum, err := controllerutils.GetChecksumOfObject(checksums)
	if err != nil {
		panic("error calculating object checksum")
	}

	return combinedChecksum
}

func calculateClusterDeploymentChecksum(clusterDeployment *hivev1.ClusterDeployment) string {
	checksum, err := controllerutils.GetChecksumOfObjects(clusterDeployment.ObjectMeta, clusterDeployment.Spec)
	if err != nil {
		panic("error calculating object checksum")
	}

	checksums := []string{checksum}

	combinedChecksum, err := controllerutils.GetChecksumOfObject(checksums)
	if err != nil {
		panic("error calculating object checksum")
	}

	return combinedChecksum
}

func checkpointBase() testcheckpoint.Option {
	return func(checkpoint *hivev1.Checkpoint) {
		checkpoint.Name = checkpointName
		checkpoint.Namespace = namespace
	}
}

func dnsZoneBase() testdnszone.Option {
	return func(dnsZone *hivev1.DNSZone) {
		dnsZone.Name = "somednszone"
		dnsZone.Namespace = namespace
	}
}

func syncSetBase() testsyncset.Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Name = "somesyncset"
		syncSet.Namespace = namespace
	}
}

func clusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Name = "someclusterdeployment"
		clusterDeployment.Namespace = namespace
	}
}

func fakeClientReconcileBackup(existingObjects []runtime.Object) *ReconcileBackup {
	return &ReconcileBackup{
		Client:                     testfake.NewFakeClientBuilder().WithRuntimeObjects(existingObjects...).Build(),
		scheme:                     scheme.GetScheme(),
		reconcileRateLimitDuration: defaultReconcileRateLimitDuration,
		logger:                     log.WithField("controller", ControllerName),
		veleroNamespace:            "velero",
	}
}

func assertErrorsAreEqualType(t *testing.T, expected, actual error) {
	if expected == nil && actual == nil {
		return
	}

	expectedType := reflect.TypeOf(expected)
	actualType := reflect.TypeOf(actual)

	// Both sides must be the same type.
	assert.Equal(t, expectedType, actualType)

	// Test for aggregate errors (and make sure they contain equal errors)
	if expectedAsAggr, isAggr := expected.(utilerrors.Aggregate); isAggr {
		assertAggregateErrorsAreEqualType(t, expectedAsAggr, actual.(utilerrors.Aggregate))
	}
}

func assertAggregateErrorsAreEqualType(t *testing.T, expected, actual utilerrors.Aggregate) {
	expectedErrors := expected.Errors()
	actualErrors := actual.Errors()

	assert.Equal(t, len(expectedErrors), len(actualErrors))

	for i := range expectedErrors {
		assert.Equal(t, reflect.TypeOf(expectedErrors[i]), reflect.TypeOf(actualErrors[i]))
	}
}

// assertObjectArraysMostlyEqual wraps AssertEqualWhereItCounts for lists of objects
func assertObjectArraysMostlyEqual(t *testing.T, expected, actual []runtime.Object) {
	assert.Equal(t, len(expected), len(actual), fmt.Sprintf("Expected (%d) and Actual (%d) are different sizes.", len(expected), len(actual)))
	for i := 0; i < len(expected); i++ {
		testassert.AssertEqualWhereItCounts(t, expected[i], actual[i], fmt.Sprintf("Mismatch in object #%d", i+1))
	}
}

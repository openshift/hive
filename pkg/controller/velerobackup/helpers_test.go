package velerobackup

import (
	"reflect"
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	"github.com/openshift/hive/pkg/test/generic"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	namespace = "notarealns"
)

var (
	errDefault = errors.New("Not a real error")
	errStatus  = kerrors.StatusError{
		ErrStatus: metav1.Status{},
	}
)

func assertHiveObjectsEqual(t *testing.T, expected, actual []*hiveObject) {
	for i := range expected {
		assert.Equal(t, expected[i].object, actual[i].object)
	}
}

func defaultChecksumFunc(object runtime.Object) string {
	hobj, err := newHiveObject(object, defaultLogger())
	if err != nil {
		panic(err)
	}

	return hobj.checksum
}

func unchangedSyncSetBase() testsyncset.Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Name = "somesyncset-unchanged"
		syncSet.Namespace = namespace
		generic.WithBackupChecksum(defaultChecksumFunc)(syncSet)
	}
}

func changedSyncSetBase() testsyncset.Option {
	return func(syncSet *hivev1.SyncSet) {
		syncSet.Name = "somesyncset-changed"
		syncSet.Namespace = namespace
	}
}

func unchangedClusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Name = "someclusterdeployment-unchanged"
		clusterDeployment.Namespace = namespace
		testgeneric.WithBackupChecksum(defaultChecksumFunc)(clusterDeployment)
	}
}

func changedClusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		clusterDeployment.Name = "someclusterdeployment-changed"
		clusterDeployment.Namespace = namespace
	}
}

func defaultLogger() log.FieldLogger {
	return log.WithField("controller", controllerName)
}

func fakeClientReconcileBackup(existingObjects []runtime.Object) *ReconcileBackup {
	return &ReconcileBackup{
		Client: fake.NewFakeClient(existingObjects...),
		scheme: scheme.Scheme,
		logger: defaultLogger(),
	}
}

func ro2ho(ro runtime.Object) *hiveObject {
	obj, err := newHiveObject(ro, defaultLogger())

	if err != nil {
		panic("This should never happen since we control the test data.")
	}

	return obj
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

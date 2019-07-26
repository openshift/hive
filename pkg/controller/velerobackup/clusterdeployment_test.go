package velerobackup

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	testclusterdeployment "github.com/openshift/hive/pkg/controller/test/clusterdeployment"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	namespace = "notarealns"
)

func unchangedClusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		testclusterdeployment.WithName("someclusterdeployment-unchanged")(clusterDeployment)
		testclusterdeployment.WithNamespace(namespace)(clusterDeployment)
		testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)(clusterDeployment)
	}
}

func changedClusterDeploymentBase() testclusterdeployment.Option {
	return func(clusterDeployment *hivev1.ClusterDeployment) {
		testclusterdeployment.WithName("someclusterdeployment-changed")(clusterDeployment)
		testclusterdeployment.WithNamespace(namespace)(clusterDeployment)
	}
}

func emptyReconcileBackup() *ReconcileBackup {
	return &ReconcileBackup{
		logger:       log.WithField("controller", controllerName),
		checksumFunc: controllerutils.GetChecksumOfObjects,
	}
}

func getHashOfObjectsFuncReturnsError(objects ...interface{}) (string, error) {
	return "", errDefault
}

func fakeClientReconcileBackup(existingObjects []runtime.Object) *ReconcileBackup {
	return &ReconcileBackup{
		Client:       fake.NewFakeClient(existingObjects...),
		scheme:       scheme.Scheme,
		logger:       log.WithField("controller", controllerName),
		checksumFunc: controllerutils.GetChecksumOfObjects,
	}
}

func newReconcileBackupUsingFunc(getHashOfObjectsFunc controllerutils.ChecksumOfObjectsFunc) *ReconcileBackup {
	retval := emptyReconcileBackup()

	if getHashOfObjectsFunc != nil {
		retval.checksumFunc = getHashOfObjectsFunc
	}
	return retval
}

func newReconcileBackupUsingFakeClient(existingObjects []runtime.Object, getHashOfObjectsFunc controllerutils.ChecksumOfObjectsFunc) *ReconcileBackup {
	tmp := newReconcileBackupUsingFunc(getHashOfObjectsFunc)

	retval := &ReconcileBackup{
		Client:       fake.NewFakeClient(existingObjects...),
		scheme:       scheme.Scheme,
		logger:       log.WithField("controller", controllerName),
		checksumFunc: tmp.checksumFunc,
	}
	return retval
}

func changedClusterDeploymentReconcileRequests() []reconcile.Request {
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: namespace,
			},
		},
	}
}

var (
	errDefault = fmt.Errorf("Not a real error")
	errStatus  = errors.StatusError{
		ErrStatus: metav1.Status{
			Message: "clusterdeployments.hive.openshift.io \"someclusterdeployment-changed-1\" not found",
		},
	}

	defaultClusterDeploymentChecksumFunc = emptyReconcileBackup().clusterDeploymentChecksum
)

func TestClusterDeploymentWatchHandler(t *testing.T) {
	tests := []struct {
		name             string
		mapObject        handler.MapObject
		expectedRequests []reconcile.Request
	}{
		{
			name: "Not ClusterDeployment passed in case",
			mapObject: handler.MapObject{
				Object: &hivev1.SyncSet{},
			},
			expectedRequests: []reconcile.Request{},
		},
		{
			name: "ClusterDeployment unchanged since last backup case",
			mapObject: handler.MapObject{
				Object: testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase()),
			},
			expectedRequests: changedClusterDeploymentReconcileRequests(),
		},
		{
			name: "ClusterDeployment changed since last backup case",
			mapObject: handler.MapObject{
				Object: testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			expectedRequests: changedClusterDeploymentReconcileRequests(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := newReconcileBackupUsingFunc(nil)

			// Act
			requests := r.clusterDeploymentWatchHandler(test.mapObject)

			// Assert
			assert.Equal(t, test.expectedRequests, requests)
		})
	}
}

func TestGetChangedInNamespace(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                       string
		getHashOfObjectsFunc       controllerutils.ChecksumOfObjectsFunc
		existingObjects            []runtime.Object
		expectedClusterDeployments []*hivev1.ClusterDeployment
		expectedError              error
	}{
		{
			name:                       "No ClusterDeployments case",
			existingObjects:            []runtime.Object{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
			expectedError:              nil,
		},
		{
			name: "No Chnaged ClusterDeployments case",
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase()),
			},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
			expectedError:              nil,
		},
		{
			name: "1 Changed ClusterDeployments case",
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase()),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			expectedError: nil,
		},
		{
			name: "Multiple Changed ClusterDeployments case",
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
			},
			expectedError: nil,
		},
		{
			name:                 "Error  case",
			getHashOfObjectsFunc: getHashOfObjectsFuncReturnsError,
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedError:              errDefault,
			expectedClusterDeployments: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := newReconcileBackupUsingFakeClient(test.existingObjects, test.getHashOfObjectsFunc)

			// Act
			actualClusterDeployments, actualError := r.getModifiedClusterDeploymentsInNamespace(namespace)

			// Assert
			assert.Equal(t, len(test.expectedClusterDeployments), len(actualClusterDeployments))
			//assert.True(t, reflect.DeepEqual(actualClusterDeployments, test.expectedClusterDeployments))
			assert.Equal(t, test.expectedClusterDeployments, actualClusterDeployments)
			assert.Equal(t, test.expectedError, actualError)
		})
	}
}

func TestUpdateLastBackupChecksum(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                       string
		getHashOfObjectsFunc       controllerutils.ChecksumOfObjectsFunc
		clusterDeployments         []*hivev1.ClusterDeployment
		existingObjects            []runtime.Object
		expectedClusterDeployments []*hivev1.ClusterDeployment
		expectedError              error
	}{
		{
			name:                       "No ClusterDeployments case",
			clusterDeployments:         []*hivev1.ClusterDeployment{},
			existingObjects:            []runtime.Object{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
			expectedError:              nil,
		},
		{
			name: "1 changed ClusterDeployment case",
			clusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedError: nil,
		},
		{
			name: "Multiple changed ClusterDeployment case",
			clusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3")),
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3")),
			},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedError: nil,
		},
		{
			name:                 "Checksum error case",
			getHashOfObjectsFunc: getHashOfObjectsFuncReturnsError,
			clusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			existingObjects:            []runtime.Object{},
			expectedError:              utilerrors.NewAggregate([]error{errDefault}),
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
		},
		{
			name: "List error case",
			clusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			existingObjects:            []runtime.Object{},
			expectedError:              utilerrors.NewAggregate([]error{&errStatus}),
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := newReconcileBackupUsingFakeClient(test.existingObjects, test.getHashOfObjectsFunc)

			// Act
			actualError := r.updateClusterDeploymentLastBackupChecksum(test.clusterDeployments)
			actualClusterDeploymentList := &hivev1.ClusterDeploymentList{}
			r.List(context.TODO(), actualClusterDeploymentList, client.InNamespace(namespace))
			actualClusterDeployments := toPtrSlice(actualClusterDeploymentList)

			// Assert
			assert.Equal(t, test.expectedClusterDeployments, actualClusterDeployments)
			assert.True(t, areErrorsEqualType(test.expectedError, actualError))
		})
	}
}

func toPtrSlice(clusterDeployments *hivev1.ClusterDeploymentList) []*hivev1.ClusterDeployment {
	retval := make([]*hivev1.ClusterDeployment, 0)

	clusterDeploymentItems := clusterDeployments.Items

	for i := range clusterDeploymentItems {
		val := clusterDeploymentItems[i]
		retval = append(retval, &val)
	}

	return retval
}

func areErrorsEqualType(lhs, rhs error) bool {
	if lhs == nil && rhs == nil {
		return true
	}

	lhsType := reflect.TypeOf(lhs)
	rhsType := reflect.TypeOf(rhs)

	if lhsType != rhsType {
		return false
	}

	// Test for aggregate errors (and make sure they contain equal errors)
	aggregateErrorType := reflect.TypeOf(utilerrors.NewAggregate([]error{errDefault}))
	if lhsType == aggregateErrorType {
		return areAggregateErrorsEqualType(lhs.(utilerrors.Aggregate), rhs.(utilerrors.Aggregate))
	}

	return false
}

func areAggregateErrorsEqualType(lhs, rhs utilerrors.Aggregate) bool {
	lhsErrors := lhs.Errors()
	rhsErrors := rhs.Errors()

	if len(lhsErrors) != len(rhsErrors) {
		return false
	}

	for i := range lhsErrors {
		if reflect.TypeOf(lhsErrors[i]) != reflect.TypeOf(rhsErrors[i]) {
			return false
		}
	}

	// If we make it here, then all comparisions succeeded.
	return true
}

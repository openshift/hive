package velerobackup

import (
	"testing"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	"github.com/openshift/hive/pkg/apis"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"
	"github.com/openshift/hive/pkg/test/generic"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		request         reconcile.Request
		existingObjects []runtime.Object
		expectedResult  reconcile.Result
		expectedError   error
		expectedObjects []runtime.Object
	}{
		{
			name: "Empty Namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{},
			},
			existingObjects: []runtime.Object{},
			expectedResult:  reconcile.Result{},
			expectedObjects: []runtime.Object{},
		},
		{
			name: "1 unchanged ClusterDeployment",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			},
		},
		{
			name: "1 changed ClusterDeployment",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
		},
		{
			name: "1 unchanged SyncSet",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testsyncset.Build(unchangedSyncSetBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testsyncset.Build(unchangedSyncSetBase()),
			},
		},
		{
			name: "1 changed SyncSet",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testsyncset.Build(changedSyncSetBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testsyncset.Build(changedSyncSetBase(), testsyncset.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
		},
		{
			name: "1 unchanged DNSZone",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testdnszone.Build(unchangedDNSZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testdnszone.Build(unchangedDNSZoneBase()),
			},
		},
		{
			name: "1 changed DNSZone",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testdnszone.Build(changedDNSZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testdnszone.Build(changedDNSZoneBase(), testdnszone.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
		},
		{
			name: "Multiple unchanged HiveObject",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
		},
		{
			name: "Multiple changed HiveObject",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
				testsyncset.Build(changedSyncSetBase()),
				testdnszone.Build(changedDNSZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testsyncset.Build(changedSyncSetBase(), testsyncset.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testdnszone.Build(changedDNSZoneBase(), testdnszone.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
		},
		{
			name: "Multiple changed / unchanged HiveObject",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(changedSyncSetBase()),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(changedDNSZoneBase()),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(changedSyncSetBase(), testsyncset.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(changedDNSZoneBase(), testdnszone.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualResult, actualError := r.Reconcile(test.request)
			actualObjects, err := getRuntimeObjects(r, typesToList, namespace)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, test.expectedResult, actualResult)
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestGetChangedInNamespace(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                string
		existingObjects     []runtime.Object
		expectedHiveObjects []*hiveObject
		expectedError       error
	}{
		{
			name:                "No Hive objects",
			existingObjects:     []runtime.Object{},
			expectedHiveObjects: []*hiveObject{},
			expectedError:       nil,
		},
		{
			name: "No Changed Hive objects",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
			expectedHiveObjects: []*hiveObject{},
			expectedError:       nil,
		},
		{
			name: "1 Changed Hive object",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testclusterdeployment.Build(changedClusterDeploymentBase()),
			},
			expectedHiveObjects: []*hiveObject{
				ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
			},
			expectedError: nil,
		},
		{
			name: "Multiple Changed Hive objects",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testsyncset.Build(changedSyncSetBase()),
				testsyncset.Build(unchangedSyncSetBase()),
				testdnszone.Build(changedDNSZoneBase()),
				testdnszone.Build(unchangedDNSZoneBase()),
			},
			expectedHiveObjects: []*hiveObject{
				ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
				ro2ho(testsyncset.Build(changedSyncSetBase())),
				ro2ho(testdnszone.Build(changedDNSZoneBase())),
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualHiveObjects, actualError := r.getModifiedHiveObjectsInNamespace(namespace)

			// Assert
			assert.Equal(t, len(test.expectedHiveObjects), len(actualHiveObjects))
			assertHiveObjectsEqual(t, test.expectedHiveObjects, actualHiveObjects)
			assert.Equal(t, test.expectedError, actualError)
		})
	}
}

func TestUpdateHiveObjectsLastBackupChecksum(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		hiveObjects     []*hiveObject
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
		expectedError   error
	}{
		{
			name:            "No HiveObjects",
			hiveObjects:     []*hiveObject{},
			existingObjects: []runtime.Object{},
			expectedObjects: []runtime.Object{},
			expectedError:   nil,
		},
		{
			name: "1 changed ClusterDeployment",
			hiveObjects: []*hiveObject{
				ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
			expectedError: nil,
		},
		{
			name: "1 changed SyncSet",
			hiveObjects: []*hiveObject{
				ro2ho(testsyncset.Build(changedSyncSetBase())),
			},
			existingObjects: []runtime.Object{
				testsyncset.Build(changedSyncSetBase()),
			},
			expectedObjects: []runtime.Object{
				testsyncset.Build(changedSyncSetBase(), testsyncset.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
			expectedError: nil,
		},
		{
			name: "1 changed DNSZone",
			hiveObjects: []*hiveObject{
				ro2ho(testdnszone.Build(changedDNSZoneBase())),
			},
			existingObjects: []runtime.Object{
				testdnszone.Build(changedDNSZoneBase()),
			},
			expectedObjects: []runtime.Object{
				testdnszone.Build(changedDNSZoneBase(), testdnszone.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
			expectedError: nil,
		},
		{
			name: "Multiple changed HiveObjects",
			hiveObjects: []*hiveObject{
				ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
				ro2ho(testsyncset.Build(changedSyncSetBase())),
				ro2ho(testdnszone.Build(changedDNSZoneBase())),
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase()),
				testsyncset.Build(changedSyncSetBase()),
				testdnszone.Build(changedDNSZoneBase()),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(changedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testsyncset.Build(changedSyncSetBase(), testsyncset.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
				testdnszone.Build(changedDNSZoneBase(), testdnszone.Generic(generic.WithBackupChecksum(defaultChecksumFunc))),
			},
			expectedError: nil,
		},
		{
			name: "List error",
			hiveObjects: []*hiveObject{
				ro2ho(testclusterdeployment.Build(changedClusterDeploymentBase())),
			},
			existingObjects: []runtime.Object{},
			expectedError:   &errStatus,
			expectedObjects: []runtime.Object{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualError := r.updateHiveObjectsLastBackupChecksum(test.hiveObjects)
			actualObjects, err := getRuntimeObjects(r, typesToList, namespace)

			// Assert
			assert.NoError(t, err)
			assert.Equal(t, test.expectedObjects, actualObjects)
			assertErrorsAreEqualType(t, test.expectedError, actualError)
		})
	}
}

func TestGetRuntimeObjects(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name            string
		existingObjects []runtime.Object
		expectedObjects []runtime.Object
	}{
		{
			name:            "Empty Namespace",
			existingObjects: []runtime.Object{},
			expectedObjects: []runtime.Object{},
		},
		{
			name: "1 ClusterDeployment",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			},
		},
		{
			name: "2 ClusterDeployments in different namespaces",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
				testclusterdeployment.Build(unchangedClusterDeploymentBase(), testclusterdeployment.Generic(generic.WithNamespace("not the correct namespace"))),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(unchangedClusterDeploymentBase()),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualObjects, actualError := getRuntimeObjects(r, typesToList, namespace)

			// Assert
			assert.NoError(t, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

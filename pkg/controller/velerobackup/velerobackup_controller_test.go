package velerobackup

import (
	"context"
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	testcheckpoint "github.com/openshift/hive/pkg/test/checkpoint"
	testclusterdeployment "github.com/openshift/hive/pkg/test/clusterdeployment"
	testdnszone "github.com/openshift/hive/pkg/test/dnszone"

	"github.com/openshift/hive/pkg/test/generic"
	testsyncset "github.com/openshift/hive/pkg/test/syncset"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
			name: "Simulate Empty Namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{},
			},
			existingObjects: emptyRuntimeObjectSlice,
			expectedResult:  reconcile.Result{},
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "Simulate first backup of a namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testdnszone.Build(dnsZoneBase()),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testdnszone.Build(dnsZoneBase()),
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
					[]runtime.Object{
						testclusterdeployment.Build(clusterDeploymentBase()),
						testsyncset.Build(syncSetBase()),
						testdnszone.Build(dnsZoneBase()),
					}))),
			},
		},
		{
			name: "Simulate no changes since last backup of a namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testcheckpoint.Build(checkpointBase(),
					testcheckpoint.WithLastBackupRef(hivev1.BackupReference{Name: "notarealbackup", Namespace: namespace}),
					testcheckpoint.WithLastBackupTime(fiveHoursAgo),
					testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
						[]runtime.Object{
							testclusterdeployment.Build(clusterDeploymentBase()),
						}))),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testcheckpoint.Build(checkpointBase(),
					testcheckpoint.WithLastBackupRef(hivev1.BackupReference{Name: "notarealbackup", Namespace: namespace}),
					testcheckpoint.WithLastBackupTime(fiveHoursAgo),
					testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
						[]runtime.Object{
							testclusterdeployment.Build(clusterDeploymentBase()),
						}))),
			},
		},
		{
			name: "Simulate adding 1 object and getting rate limited",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(),
					testcheckpoint.WithLastBackupRef(hivev1.BackupReference{Name: "notarealbackup", Namespace: namespace}),
					testcheckpoint.WithLastBackupTime(twoMinutesAgo),
					testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
						[]runtime.Object{
							testclusterdeployment.Build(clusterDeploymentBase()),
						}))),
			},
			expectedResult: reconcile.Result{
				RequeueAfter: ((3 * time.Minute) - twoMinuteDuration),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(),
					testcheckpoint.WithLastBackupRef(hivev1.BackupReference{Name: "notarealbackup", Namespace: namespace}),
					testcheckpoint.WithLastBackupTime(twoMinutesAgo),
					testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
						[]runtime.Object{
							testclusterdeployment.Build(clusterDeploymentBase()),
						}))),
			},
		},
		{
			name: "Simulate adding 1 object since last backup of a namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
					[]runtime.Object{
						testclusterdeployment.Build(clusterDeploymentBase()),
					}))),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
					[]runtime.Object{
						testclusterdeployment.Build(clusterDeploymentBase()),
						testsyncset.Build(syncSetBase()),
					}))),
			},
		},
		{
			name: "Simulate removing 1 object since last backup of a namespace",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
					[]runtime.Object{
						testclusterdeployment.Build(clusterDeploymentBase()),
					}))),
			},
			expectedResult: reconcile.Result{},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testsyncset.Build(syncSetBase()),
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum(calculateRuntimeObjectsChecksum(
					[]runtime.Object{
						testclusterdeployment.Build(clusterDeploymentBase()),
						testsyncset.Build(syncSetBase()),
					}))),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)
			types := append(hiveNamespaceScopedListTypes, &hivev1.CheckpointList{})
			tolerance := float64(10 * time.Second)

			// Act
			actualResult, actualError := r.Reconcile(test.request)
			actualObjects, err := r.getRuntimeObjects(types, namespace)
			lastBackupName, lastBackupTimestamp := ignoreUncomparedFields(test.expectedObjects, actualObjects)

			// Assert
			assert.NoError(t, err)
			assert.NotEmpty(t, lastBackupName)
			assert.NotEmpty(t, lastBackupTimestamp)
			assert.Equal(t, test.expectedResult.Requeue, actualResult.Requeue)
			assert.InDelta(t, test.expectedResult.RequeueAfter, actualResult.RequeueAfter, tolerance)
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestCreateVeleroBackupObject(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	// Arrange
	formatStr := "2006-01-02t15-04-05z"
	actualBackups := &velerov1.BackupList{}
	r := fakeClientReconcileBackup(emptyRuntimeObjectSlice)
	timestamp := metav1.NewTime(time.Now().UTC())
	expectedBackupRef := hivev1.BackupReference{
		Name:      fmt.Sprintf("backup-%v-%v", namespace, timestamp.Format(formatStr)),
		Namespace: "velero",
	}

	// Act
	actualBackupRef, actualError := r.createVeleroBackupObject(namespace, timestamp)
	r.List(context.TODO(), actualBackups)

	// Assert
	assert.NoError(t, actualError)
	assert.Equal(t, expectedBackupRef, actualBackupRef)
	assert.Equal(t, namespace, actualBackups.Items[0].Spec.IncludedNamespaces[0])
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
			existingObjects: emptyRuntimeObjectSlice,
			expectedObjects: emptyRuntimeObjectSlice,
		},
		{
			name: "1 ClusterDeployment",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
			},
		},
		{
			name: "2 ClusterDeployments in different namespaces",
			existingObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
				testclusterdeployment.Build(clusterDeploymentBase(), testclusterdeployment.Generic(generic.WithNamespace("not the correct namespace"))),
			},
			expectedObjects: []runtime.Object{
				testclusterdeployment.Build(clusterDeploymentBase()),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualObjects, actualError := r.getRuntimeObjects(hiveNamespaceScopedListTypes, namespace)

			// Assert
			assert.NoError(t, actualError)
			assert.Equal(t, test.expectedObjects, actualObjects)
		})
	}
}

func TestGetNamespaceCheckpoint(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name               string
		existingObjects    []runtime.Object
		expectedCheckpoint *hivev1.Checkpoint
		expectedFound      bool
		expectedError      error
	}{
		{
			name:               "No Checkpoint",
			expectedFound:      false,
			existingObjects:    emptyRuntimeObjectSlice,
			expectedCheckpoint: testcheckpoint.Build(checkpointBase()),
			expectedError:      nil,
		},
		{
			name:               "Existing Checkpoint",
			expectedFound:      true,
			existingObjects:    []runtime.Object{testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL"))},
			expectedCheckpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL")),
			expectedError:      nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualCheckpoint, actualFound, actualError := r.getNamespaceCheckpoint(namespace, r.logger)

			// Assert
			assert.Equal(t, test.expectedCheckpoint, actualCheckpoint)
			assert.Equal(t, test.expectedFound, actualFound)
			assertErrorsAreEqualType(t, test.expectedError, actualError)
		})
	}
}

func TestCreateOrUpdateNamespaceCheckpoint(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name               string
		existingObjects    []runtime.Object
		found              bool
		checkpoint         *hivev1.Checkpoint
		expectedCheckpoint *hivev1.Checkpoint
		expectedError      error
	}{
		{
			name:               "Create Checkpoint",
			found:              false,
			checkpoint:         testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL")),
			existingObjects:    emptyRuntimeObjectSlice,
			expectedCheckpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL")),
			expectedError:      nil,
		},
		{
			name:       "Update Checkpoint",
			found:      true,
			checkpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-AFTER")),
			existingObjects: []runtime.Object{
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-BEFORE")),
			},
			expectedCheckpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-AFTER")),
			expectedError:      nil,
		},
		{
			name:       "Error Creating",
			found:      false,
			checkpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-AFTER")),
			existingObjects: []runtime.Object{
				testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-BEFORE")),
			},
			expectedCheckpoint: testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-BEFORE")),
			expectedError:      statusErr,
		},
		{
			name:               "Error Updating",
			found:              true,
			checkpoint:         testcheckpoint.Build(checkpointBase(), testcheckpoint.WithLastBackupChecksum("NOTREAL-AFTER")),
			existingObjects:    emptyRuntimeObjectSlice,
			expectedCheckpoint: &hivev1.Checkpoint{},
			expectedError:      statusErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			actualCheckpoint := &hivev1.Checkpoint{}
			namespacedName := types.NamespacedName{Namespace: namespace, Name: checkpointName}
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualError := r.createOrUpdateNamespaceCheckpoint(test.checkpoint, test.found, r.logger)
			r.Get(context.TODO(), namespacedName, actualCheckpoint)

			// Assert
			assert.Equal(t, test.expectedCheckpoint, actualCheckpoint)
			assertErrorsAreEqualType(t, test.expectedError, actualError)
		})
	}
}

func TestCalculateObjectsChecksumWithoutStatus(t *testing.T) {
	tests := []struct {
		name             string
		object           runtime.Object
		expectedChecksum string
	}{
		{
			name:             "Valid ClusterDeployment Checksum",
			object:           testclusterdeployment.Build(clusterDeploymentBase()),
			expectedChecksum: calculateClusterDeploymentChecksum(testclusterdeployment.Build(clusterDeploymentBase())),
		},
		{
			name:             "Invalid type (not a hive object)",
			object:           &corev1.Pod{},
			expectedChecksum: calculateErrorChecksum(),
		},
		{
			name:             "Invalid nil object",
			object:           nil,
			expectedChecksum: calculateErrorChecksum(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(emptyRuntimeObjectSlice)

			// Act
			actualChecksum := r.calculateObjectsChecksumWithoutStatus(r.logger, test.object)

			// Assert
			assert.Equal(t, test.expectedChecksum, actualChecksum)
		})
	}
}

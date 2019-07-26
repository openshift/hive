package velerobackup

import (
	"context"
	"testing"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	"github.com/openshift/hive/pkg/apis"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	testclusterdeployment "github.com/openshift/hive/pkg/controller/test/clusterdeployment"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcile(t *testing.T) {
	apis.AddToScheme(scheme.Scheme)
	velerov1.AddToScheme(scheme.Scheme)

	tests := []struct {
		name                       string
		request                    reconcile.Request
		existingObjects            []runtime.Object
		expectedResult             reconcile.Result
		expectedError              error
		expectedClusterDeployments []*hivev1.ClusterDeployment
	}{
		{
			name: "Empty Namespace case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{},
			},
			existingObjects:            []runtime.Object{},
			expectedResult:             reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{},
		},
		{
			name: "1 unchanged ClusterDeployment case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase()),
			},
			expectedResult: reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase()),
			},
		},
		{
			name: "1 changed ClusterDeployment case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase()),
			},
			expectedResult: reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
		},
		{
			name: "Multiple unchanged ClusterDeployment case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedResult: reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
		},
		{
			name: "Multiple changed ClusterDeployment case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3")),
			},
			expectedResult: reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
		},
		{
			name: "Multiple changed / unchanged ClusterDeployment case",
			request: reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: namespace,
				},
			},
			existingObjects: []runtime.Object{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3")),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
			expectedResult: reconcile.Result{},
			expectedClusterDeployments: []*hivev1.ClusterDeployment{
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("1"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("2"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(changedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
				testclusterdeployment.BuildClusterDeployment(unchangedClusterDeploymentBase(), testclusterdeployment.WithNamePostfix("3"), testclusterdeployment.WithBackupChecksum(defaultClusterDeploymentChecksumFunc)),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			r := fakeClientReconcileBackup(test.existingObjects)

			// Act
			actualResult, actualError := r.Reconcile(test.request)
			actualClusterDeploymentList := &hivev1.ClusterDeploymentList{}
			r.List(context.TODO(), actualClusterDeploymentList, client.InNamespace(namespace))
			actualClusterDeployments := toPtrSlice(actualClusterDeploymentList)

			// Assert
			assert.Equal(t, test.expectedResult, actualResult)
			assert.Equal(t, test.expectedError, actualError)
			assert.Equal(t, test.expectedClusterDeployments, actualClusterDeployments)
		})
	}
}

package clusterpool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/apis/hive/v1/aws"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsecret "github.com/openshift/hive/pkg/test/secret"
)

const (
	testNamespace     = "test-namespace"
	hiveNamespace     = "hive"
	testLeasePoolName = "aws-us-east-1"
	credsSecretName   = "aws-creds"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

func TestReconcileClusterPool(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)

	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme)
	}
	unclaimedCDBuilder := func(name string) testcd.Builder {
		return cdBuilder(name).Options(
			testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, hivev1.ClusterPoolStateUnclaimed),
		)
	}
	secretBuilder := testsecret.FullBuilder(testNamespace, credsSecretName, scheme).Options(
		testsecret.WithDataKeyValue("dummykey", []byte("dummyval")),
	)

	tests := []struct {
		name                      string
		existing                  []runtime.Object
		expectedTotalClusters     int
		expectedUnclaimedClusters int
		expectedObservedSize      int32
		expectedObservedReady     int32
		expectedDeletedClusters   []string
		expectFinalizerRemoved    bool
	}{
		{
			name: "create all clusters",
			existing: []runtime.Object{
				buildPool(5),
				secretBuilder.Build(),
			},
			expectedTotalClusters:     5,
			expectedUnclaimedClusters: 5,
			expectedObservedSize:      0,
			expectedObservedReady:     0,
		},
		{
			name: "scale up",
			existing: []runtime.Object{
				buildPool(5),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:     5,
			expectedUnclaimedClusters: 5,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
		{
			name: "scale down",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				unclaimedCDBuilder("c5").Build(testcd.Installed()),
				unclaimedCDBuilder("c6").Build(testcd.Installed()),
			},
			expectedTotalClusters:     3,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      6,
			expectedObservedReady:     6,
		},
		{
			name: "delete installing clusters first",
			existing: []runtime.Object{
				buildPool(1),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters:     1,
			expectedUnclaimedClusters: 1,
			expectedObservedSize:      2,
			expectedObservedReady:     1,
			expectedDeletedClusters:   []string{"c2"},
		},
		{
			name: "delete most recent installing clusters first",
			existing: []runtime.Object{
				buildPool(1),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
				unclaimedCDBuilder("c2").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 2, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
			},
			expectedTotalClusters:     1,
			expectedUnclaimedClusters: 1,
			expectedObservedSize:      2,
			expectedObservedReady:     0,
			expectedDeletedClusters:   []string{"c2"},
		},
		{
			name: "delete installed clusters when there are not enough installing to delete",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				unclaimedCDBuilder("c5").Build(testcd.Installed()),
				unclaimedCDBuilder("c6").Build(),
			},
			expectedTotalClusters:     3,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      6,
			expectedObservedReady:     4,
			expectedDeletedClusters:   []string{"c3", "c6"},
		},
		{
			name: "clusters deleted when clusterpool deleted",
			existing: []runtime.Object{
				func() runtime.Object {
					p := buildPool(3)
					now := metav1.Now()
					p.DeletionTimestamp = &now
					return p
				}(),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:     0,
			expectedUnclaimedClusters: 0,
			expectFinalizerRemoved:    true,
		},
		{
			name: "finalizer added to clusterpool",
			existing: []runtime.Object{
				func() runtime.Object {
					p := buildPool(3)
					p.Finalizers = nil
					return p
				}(),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:     3,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
		{
			name: "clusters not part of pool are not counted against pool size",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(),
			},
			expectedTotalClusters:     4,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
		{
			name: "claimed clusters are not counted against pool size",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, hivev1.ClusterPoolStateClaimed),
				),
			},
			expectedTotalClusters:     4,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
		{
			name: "clusters in different pool are not counted against pool size",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, "other-pool", hivev1.ClusterPoolStateUnclaimed),
				),
			},
			expectedTotalClusters:     4,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
		{
			name: "deleting clusters are not counted against pool size",
			existing: []runtime.Object{
				buildPool(3),
				secretBuilder.Build(),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				cdBuilder("c5").GenericOptions(testgeneric.Deleted()).Build(),
			},
			expectedTotalClusters:     5,
			expectedUnclaimedClusters: 3,
			expectedObservedSize:      3,
			expectedObservedReady:     2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme, test.existing...)
			rcp := &ReconcileClusterPool{
				Client: fakeClient,
				logger: log.WithFields(nil),
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testLeasePoolName,
					Namespace: testNamespace,
				},
			}

			_, err := rcp.Reconcile(reconcileRequest)
			require.NoError(t, err, "expected no error from reconcile")

			cds := &hivev1.ClusterDeploymentList{}
			err = fakeClient.List(context.Background(), cds)
			require.NoError(t, err)

			assert.Len(t, cds.Items, test.expectedTotalClusters, "unexpected number of total clusters")

			actualUnclaimedClusters := 0
			poolRef := hivev1.ClusterPoolReference{
				Namespace: testNamespace,
				Name:      testLeasePoolName,
				State:     hivev1.ClusterPoolStateUnclaimed,
			}
			for _, cd := range cds.Items {
				if cd.Spec.ClusterPoolRef != nil && *cd.Spec.ClusterPoolRef == poolRef {
					actualUnclaimedClusters++
				}
			}
			assert.Equal(t, test.expectedUnclaimedClusters, actualUnclaimedClusters, "unexpected number of unclaimed clusters")

			for _, expectedDeletedName := range test.expectedDeletedClusters {
				for _, cd := range cds.Items {
					assert.NotEqual(t, expectedDeletedName, cd.Name, "expected cluster to have been deleted")
				}
			}

			pool := &hivev1.ClusterPool{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeasePoolName}, pool)
			assert.NoError(t, err, "unexpected error getting clusterpool")
			if test.expectFinalizerRemoved {
				assert.NotContains(t, pool.Finalizers, finalizer, "expected no finalizer on clusterpool")
			} else {
				assert.Contains(t, pool.Finalizers, finalizer, "expect finalizer on clusterpool")
				assert.Equal(t, test.expectedObservedSize, pool.Status.Size, "unexpected observed size")
				assert.Equal(t, test.expectedObservedReady, pool.Status.Ready, "unexpected observed ready count")
			}
		})
	}
}

func buildPool(size int32) *hivev1.ClusterPool {
	return &hivev1.ClusterPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       testLeasePoolName,
			Namespace:  testNamespace,
			Finalizers: []string{finalizer},
		},
		Spec: hivev1.ClusterPoolSpec{
			Platform: hivev1.Platform{
				AWS: &aws.Platform{
					CredentialsSecretRef: v1.LocalObjectReference{Name: credsSecretName},
					Region:               "us-east-1",
				},
			},
			Size:       size,
			BaseDomain: "devclusters.example.com",
		},
	}
}

func buildSecret(secretType corev1.SecretType, namespace, name, key, value string) *corev1.Secret {
	s := &corev1.Secret{
		Type: secretType,
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: []byte(value),
		},
	}
	return s
}

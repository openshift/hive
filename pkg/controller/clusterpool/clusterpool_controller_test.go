package clusterpool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcp "github.com/openshift/hive/pkg/test/clusterpool"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testsecret "github.com/openshift/hive/pkg/test/secret"
)

const (
	testNamespace     = "test-namespace"
	testLeasePoolName = "aws-us-east-1"
	credsSecretName   = "aws-creds"
	imageSetName      = "test-image-set"
)

func TestReconcileClusterPool(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)

	poolBuilder := testcp.FullBuilder(testNamespace, testLeasePoolName, scheme).
		GenericOptions(
			testgeneric.WithFinalizer(finalizer),
		).
		Options(
			testcp.ForAWS(credsSecretName, "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet(imageSetName),
		)
	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).Options(
			testcd.WithPowerState(hivev1.HibernatingClusterPowerState),
		)
	}
	unclaimedCDBuilder := func(name string) testcd.Builder {
		return cdBuilder(name).Options(
			testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
		)
	}

	tests := []struct {
		name                               string
		existing                           []runtime.Object
		noClusterImageSet                  bool
		noCredsSecret                      bool
		expectError                        bool
		expectedTotalClusters              int
		expectedObservedSize               int32
		expectedObservedReady              int32
		expectedDeletedClusters            []string
		expectFinalizerRemoved             bool
		expectedMissingDependenciesStatus  *bool
		expectedMissingDependenciesMessage string
		expectedAssignedClaims             int
		expectedUnassignedClaims           int
		expectedLabels                     map[string]string // Tested on all clusters, so will not work if your test has pre-existing cds in the pool.
	}{
		{
			name: "create all clusters",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(5), testcp.WithClusterDeploymentLabels(map[string]string{"foo": "bar"})),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  0,
			expectedObservedReady: 0,
			expectedLabels:        map[string]string{"foo": "bar"},
		},
		{
			name: "scale up",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(5)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "scale down",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				unclaimedCDBuilder("c5").Build(testcd.Installed()),
				unclaimedCDBuilder("c6").Build(testcd.Installed()),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  6,
			expectedObservedReady: 6,
		},
		{
			name: "delete installing clusters first",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c2"},
		},
		{
			name: "delete most recent installing clusters first",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 1, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
				unclaimedCDBuilder("c2").GenericOptions(
					testgeneric.WithCreationTimestamp(time.Date(2020, 2, 2, 3, 4, 5, 6, time.UTC)),
				).Build(),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedObservedReady:   0,
			expectedDeletedClusters: []string{"c2"},
		},
		{
			name: "delete installed clusters when there are not enough installing to delete",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				unclaimedCDBuilder("c5").Build(testcd.Installed()),
				unclaimedCDBuilder("c6").Build(),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    6,
			expectedObservedReady:   4,
			expectedDeletedClusters: []string{"c3", "c6"},
		},
		{
			name: "clusters deleted when clusterpool deleted",
			existing: []runtime.Object{
				poolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  0,
			expectFinalizerRemoved: true,
		},
		{
			name: "finalizer added to clusterpool",
			existing: []runtime.Object{
				poolBuilder.GenericOptions(testgeneric.WithoutFinalizer(finalizer)).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "clusters not part of pool are not counted against pool size",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "claimed clusters are not counted against pool size",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "clusters in different pool are not counted against pool size",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, "other-pool", "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "deleting clusters are not counted against pool size",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				cdBuilder("c5").GenericOptions(testgeneric.Deleted()).Build(),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "missing ClusterImageSet",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found`,
		},
		{
			name: "missing creds secret",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `credentials secret: secrets "aws-creds" not found`,
		},
		{
			name: "missing ClusterImageSet",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found`,
		},
		{
			name: "multiple missing dependents",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `[cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found, credentials secret: secrets "aws-creds" not found]`,
		},
		{
			name: "missing dependents resolved",
			existing: []runtime.Object{
				poolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolMissingDependenciesCondition,
						Status: corev1.ConditionTrue,
					}),
				),
			},
			expectedMissingDependenciesStatus:  pointer.BoolPtr(false),
			expectedMissingDependenciesMessage: "Dependencies verified",
			expectedTotalClusters:              1,
			expectedObservedSize:               0,
			expectedObservedReady:              0,
		},
		{
			name: "with pull secret",
			existing: []runtime.Object{
				poolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
				testsecret.FullBuilder(testNamespace, "test-pull-secret", scheme).
					Build(testsecret.WithDataKeyValue(".dockerconfigjson", []byte("test docker config data"))),
			},
			expectedTotalClusters: 1,
			expectedObservedSize:  0,
			expectedObservedReady: 0,
		},
		{
			name: "missing pull secret",
			existing: []runtime.Object{
				poolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `pull secret: secrets "test-pull-secret" not found`,
		},
		{
			name: "pull secret missing docker config",
			existing: []runtime.Object{
				poolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
				testsecret.FullBuilder(testNamespace, "test-pull-secret", scheme).Build(),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  pointer.BoolPtr(true),
			expectedMissingDependenciesMessage: `pull secret: pull secret does not contain .dockerconfigjson data`,
		},
		{
			name: "assign to claim",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:    4,
			expectedObservedSize:     3,
			expectedObservedReady:    2,
			expectedAssignedClaims:   1,
			expectedUnassignedClaims: 0,
		},
		{
			name: "no ready clusters to assign to claim",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:    4,
			expectedObservedSize:     3,
			expectedObservedReady:    0,
			expectedAssignedClaims:   0,
			expectedUnassignedClaims: 1,
		},
		{
			name: "assign to multiple claims",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim-1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim-2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim-3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:    6,
			expectedObservedSize:     3,
			expectedObservedReady:    2,
			expectedAssignedClaims:   2,
			expectedUnassignedClaims: 1,
		},
		{
			name: "do not assign to claims for other pools",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool("other-pool")),
			},
			expectedTotalClusters:    3,
			expectedObservedSize:     3,
			expectedObservedReady:    2,
			expectedAssignedClaims:   0,
			expectedUnassignedClaims: 1,
		},
		{
			name: "do not assign to claims in other namespaces",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder("other-namespace", "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:    3,
			expectedObservedSize:     3,
			expectedObservedReady:    2,
			expectedAssignedClaims:   0,
			expectedUnassignedClaims: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.noClusterImageSet {
				test.existing = append(
					test.existing,
					&hivev1.ClusterImageSet{
						ObjectMeta: metav1.ObjectMeta{Name: imageSetName},
						Spec:       hivev1.ClusterImageSetSpec{ReleaseImage: "test-release-image"},
					},
				)
			}
			if !test.noCredsSecret {
				test.existing = append(
					test.existing, testsecret.FullBuilder(testNamespace, credsSecretName, scheme).
						Build(testsecret.WithDataKeyValue("dummykey", []byte("dummyval"))),
				)
			}
			fakeClient := fake.NewFakeClientWithScheme(scheme, test.existing...)
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcp := &ReconcileClusterPool{
				Client:       fakeClient,
				logger:       logger,
				expectations: controllerExpectations,
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testLeasePoolName,
					Namespace: testNamespace,
				},
			}

			_, err := rcp.Reconcile(reconcileRequest)
			if test.expectError {
				assert.Error(t, err, "expected error from reconcile")
			} else {
				assert.NoError(t, err, "expected no error from reconcile")
			}

			cds := &hivev1.ClusterDeploymentList{}
			err = fakeClient.List(context.Background(), cds)
			require.NoError(t, err)

			assert.Len(t, cds.Items, test.expectedTotalClusters, "unexpected number of total clusters")

			for _, expectedDeletedName := range test.expectedDeletedClusters {
				for _, cd := range cds.Items {
					assert.NotEqual(t, expectedDeletedName, cd.Name, "expected cluster to have been deleted")
				}
			}

			for _, cd := range cds.Items {
				assert.Equal(t, hivev1.HibernatingClusterPowerState, cd.Spec.PowerState, "expected cluster to be hibernating")
				if test.expectedLabels != nil {
					for k, v := range test.expectedLabels {
						assert.Equal(t, v, cd.Labels[k])
					}
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

			missingDependentsCondition := controllerutils.FindClusterPoolCondition(pool.Status.Conditions, hivev1.ClusterPoolMissingDependenciesCondition)
			if test.expectedMissingDependenciesStatus == nil {
				assert.Nil(t, missingDependentsCondition, "expected no MissingDependencies condition")
			} else {
				expectedStatus := corev1.ConditionFalse
				if *test.expectedMissingDependenciesStatus {
					expectedStatus = corev1.ConditionTrue
				}
				assert.Equal(t, expectedStatus, missingDependentsCondition.Status, "expected MissingDependencies condition to be true")
				assert.Equal(t, test.expectedMissingDependenciesMessage, missingDependentsCondition.Message, "unexpected MissingDependencies conditon message")
			}

			claims := &hivev1.ClusterClaimList{}
			err = fakeClient.List(context.Background(), claims)
			require.NoError(t, err)

			actualAssignedClaims := 0
			actualUnassignedClaims := 0
			for _, claim := range claims.Items {
				if claim.Spec.Namespace == "" {
					actualUnassignedClaims++
				} else {
					actualAssignedClaims++
				}
			}
			assert.Equal(t, test.expectedAssignedClaims, actualAssignedClaims, "unexpected number of assigned claims")
			assert.Equal(t, test.expectedUnassignedClaims, actualUnassignedClaims, "unexpected number of unassigned claims")
		})
	}
}

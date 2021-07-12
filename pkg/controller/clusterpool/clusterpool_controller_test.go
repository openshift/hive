package clusterpool

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcp "github.com/openshift/hive/pkg/test/clusterpool"
	"github.com/openshift/hive/pkg/test/generic"
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
	rbacv1.AddToScheme(scheme)

	poolBuilder := testcp.FullBuilder(testNamespace, testLeasePoolName, scheme).
		GenericOptions(
			testgeneric.WithFinalizer(finalizer),
		).
		Options(
			testcp.ForAWS(credsSecretName, "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet(imageSetName),
		)
	initializedPoolBuilder := poolBuilder.Options(testcp.WithCondition(hivev1.ClusterPoolCondition{
		Status: corev1.ConditionUnknown,
		Type:   hivev1.ClusterPoolMissingDependenciesCondition,
	}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolCapacityAvailableCondition,
		}),
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
		expectedMissingDependenciesStatus  corev1.ConditionStatus
		expectedCapacityStatus             corev1.ConditionStatus
		expectedMissingDependenciesMessage string
		expectedAssignedClaims             int
		expectedUnassignedClaims           int
		expectedLabels                     map[string]string // Tested on all clusters, so will not work if your test has pre-existing cds in the pool.
	}{
		{
			name: "initialize conditions",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			expectedMissingDependenciesStatus: corev1.ConditionUnknown,
			expectedCapacityStatus:            corev1.ConditionUnknown,
		},
		{
			name: "create all clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithClusterDeploymentLabels(map[string]string{"foo": "bar"})),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  0,
			expectedObservedReady: 0,
			expectedLabels:        map[string]string{"foo": "bar"},
		},
		{
			name: "scale up",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "scale up with no more capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				cdBuilder("c1").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test")),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				cdBuilder("c1").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test")),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "scale up with one more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "scale up with max concurrent and max size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  3,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 3",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "no scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").GenericOptions(generic.Deleted()).Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
			expectedObservedReady: 1,
		},
		{
			name: "no scale up with max concurrent and some deleting claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				cdBuilder("c1").GenericOptions(generic.Deleted()).Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").GenericOptions(generic.Deleted()).Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  2,
			expectedObservedReady: 1,
		},
		{
			name: "scale down",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
			name: "scale down with max concurrent enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(3)),
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
			name: "scale down with max concurrent not enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				unclaimedCDBuilder("c5").Build(testcd.Installed()),
				unclaimedCDBuilder("c6").Build(testcd.Installed()),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  6,
			expectedObservedReady: 6,
		},
		{
			name: "delete installing clusters first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
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
				initializedPoolBuilder.Build(testcp.WithSize(1)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.GenericOptions(testgeneric.WithoutFinalizer(finalizer)).Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found`,
		},
		{
			name: "missing creds secret",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `credentials secret: secrets "aws-creds" not found`,
		},
		{
			name: "missing ClusterImageSet",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found`,
		},
		{
			name: "multiple missing dependents",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
			},
			noClusterImageSet:                  true,
			noCredsSecret:                      true,
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `[cluster image set: clusterimagesets.hive.openshift.io "test-image-set" not found, credentials secret: secrets "aws-creds" not found]`,
		},
		{
			name: "missing dependents resolved",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolMissingDependenciesCondition,
						Status: corev1.ConditionTrue,
					}),
				),
			},
			expectedMissingDependenciesStatus:  corev1.ConditionFalse,
			expectedMissingDependenciesMessage: "Dependencies verified",
			expectedTotalClusters:              1,
			expectedObservedSize:               0,
			expectedObservedReady:              0,
		},
		{
			name: "max size should include the deleting unclaimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithMaxSize(3),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolCapacityAvailableCondition,
						Status: corev1.ConditionFalse,
					}),
				),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").GenericOptions(generic.Deleted()).Build(testcd.Installed()),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "max capacity resolved",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(2),
					testcp.WithMaxSize(3),
					testcp.WithCondition(hivev1.ClusterPoolCondition{
						Type:   hivev1.ClusterPoolCapacityAvailableCondition,
						Status: corev1.ConditionFalse,
					}),
				),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
			},
			expectedTotalClusters:  2,
			expectedObservedSize:   2,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "with pull secret",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
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
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `pull secret: secrets "test-pull-secret" not found`,
		},
		{
			name: "pull secret missing docker config",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(1),
					testcp.WithPullSecret("test-pull-secret"),
				),
				testsecret.FullBuilder(testNamespace, "test-pull-secret", scheme).Build(),
			},
			expectError:                        true,
			expectedMissingDependenciesStatus:  corev1.ConditionTrue,
			expectedMissingDependenciesMessage: `pull secret: pull secret does not contain .dockerconfigjson data`,
		},
		{
			name: "assign to claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
		{
			name: "do not delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
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
			name: "do not delete previously claimed clusters 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.Deleted(), testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
		},
		{
			name: "delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "deleting previously claimed clusters should use max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "deleting previously claimed clusters should use max concurrent 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  2,
			expectedObservedReady: 1,
		},
		{
			name: "deleting previously claimed clusters should use max concurrent 3",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent all used by deleting previously claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4", "c5"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent all used by installing one cluster and deleting one previously claimed cluster",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by deleting previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   3,
			expectedDeletedClusters: []string{"c4", "c5"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by one installing cluster and deleting one previously claimed cluster",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
				cdBuilder("c5").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   4,
			expectedObservedSize:    3,
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4"},
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

			_, err := rcp.Reconcile(context.TODO(), reconcileRequest)
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

			if test.expectFinalizerRemoved {
				assert.True(t, apierrors.IsNotFound(err), "expected pool to be deleted")
			} else {
				assert.NoError(t, err, "unexpected error getting clusterpool")
				assert.Contains(t, pool.Finalizers, finalizer, "expect finalizer on clusterpool")
				assert.Equal(t, test.expectedObservedSize, pool.Status.Size, "unexpected observed size")
				assert.Equal(t, test.expectedObservedReady, pool.Status.Ready, "unexpected observed ready count")
			}

			if test.expectedMissingDependenciesStatus != "" {
				missingDependenciesCondition := controllerutils.FindClusterPoolCondition(pool.Status.Conditions, hivev1.ClusterPoolMissingDependenciesCondition)
				if assert.NotNil(t, missingDependenciesCondition, "did not find MissingDependencies condition") {
					assert.Equal(t, test.expectedMissingDependenciesStatus, missingDependenciesCondition.Status,
						"unexpected MissingDependencies conditon status")
					if test.expectedMissingDependenciesMessage != "" {
						assert.Equal(t, test.expectedMissingDependenciesMessage, missingDependenciesCondition.Message,
							"unexpected MissingDependencies conditon message")
					}
				}
			}
			if test.expectedCapacityStatus != "" {
				capacityAvailableCondition := controllerutils.FindClusterPoolCondition(pool.Status.Conditions, hivev1.ClusterPoolCapacityAvailableCondition)
				if assert.NotNil(t, capacityAvailableCondition, "did not find CapacityAvailable condition") {
					assert.Equal(t, test.expectedCapacityStatus, capacityAvailableCondition.Status,
						"unexpected CapacityAvailable conditon status")
				}
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

func TestReconcileRBAC(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)

	tests := []struct {
		name string

		existing []runtime.Object

		expectedBindings []rbacv1.RoleBinding
		expectedErr      string
	}{{
		name:     "no binding referring to cluster role 1",
		existing: []runtime.Object{},
	}, {
		name: "no binding referring to cluster role 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 1",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-namespace-1",
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role but no namespace for pool 3",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-namespace-1",
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "some-namespace-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: "some-other-pool"},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role with one namespace for pool 4",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "binding referring to cluster role with multiple namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-2",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "multiple binding referring to cluster role with one namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "multiple binding referring to cluster role with multiple namespace for pool",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-2",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-2",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that is same",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are same",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that are different 1",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role binding that are different 2",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 3",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 4",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}, {
		name: "pre existing role bindings that are different 5",
		existing: []runtime.Object{
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cluster-1",
					Labels: map[string]string{constants.ClusterPoolNameLabel: testLeasePoolName},
				},
			},
			&rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-1"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},

		expectedBindings: []rbacv1.RoleBinding{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-cluster-1",
					Name:      "hive-cluster-pool-admin-binding",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}, {Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test",
				},
				Subjects: []rbacv1.Subject{{Kind: "User", Name: "test-admin-another"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      "test-2",
				},
				Subjects: []rbacv1.Subject{{Kind: "Group", Name: "test-admin-group-1"}},
				RoleRef:  rbacv1.RoleRef{Kind: "ClusterRole", Name: "hive-cluster-pool-admin"},
			},
		},
	}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewFakeClientWithScheme(scheme, test.existing...)
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			controllerExpectations := controllerutils.NewExpectations(logger)
			rcp := &ReconcileClusterPool{
				Client:       fakeClient,
				logger:       logger,
				expectations: controllerExpectations,
			}

			err := rcp.reconcileRBAC(&hivev1.ClusterPool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testLeasePoolName,
					Namespace: testNamespace,
				},
			}, logger)
			if test.expectedErr == "" {
				require.NoError(t, err)

				rbs := &rbacv1.RoleBindingList{}
				err = fakeClient.List(context.Background(), rbs)
				require.NoError(t, err)
				sort.Slice(rbs.Items, func(i, j int) bool {
					return rbs.Items[i].Namespace < rbs.Items[j].Namespace && rbs.Items[i].Name < rbs.Items[j].Name
				})
				for idx := range rbs.Items {
					rbs.Items[idx].TypeMeta = metav1.TypeMeta{}
					rbs.Items[idx].ResourceVersion = ""
				}

				assert.Equal(t, test.expectedBindings, rbs.Items)
			} else {
				require.Regexp(t, err, test.expectedErr)
			}
		})
	}
}

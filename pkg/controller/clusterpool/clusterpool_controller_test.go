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
	"github.com/openshift/hive/apis/hive/v1/aws"
	"github.com/openshift/hive/pkg/constants"
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
	rbacv1.AddToScheme(scheme)

	// See calculatePoolVersion. If this changes, the easiest way to figure out the new value is
	// to pull it from the test failure :)
	initialPoolVersion := "3533907469253ac5"

	poolBuilder := testcp.FullBuilder(testNamespace, testLeasePoolName, scheme).
		GenericOptions(
			testgeneric.WithFinalizer(finalizer),
		).
		Options(
			testcp.ForAWS(credsSecretName, "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet(imageSetName),
		)
	initializedPoolBuilder := poolBuilder.Options(
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolMissingDependenciesCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolCapacityAvailableCondition,
		}),
		testcp.WithCondition(hivev1.ClusterPoolCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterPoolAllClustersCurrentCondition,
		}),
	)
	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).Options(
			testcd.WithPowerState(hivev1.ClusterPowerStateHibernating),
			testcd.WithPoolVersion(initialPoolVersion),
		)
	}
	unclaimedCDBuilder := func(name string) testcd.Builder {
		return cdBuilder(name).Options(
			testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
		)
	}

	nowish := time.Now()

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
		expectedCDCurrentStatus            corev1.ConditionStatus
		expectedMissingDependenciesMessage string
		expectedAssignedClaims             int
		expectedUnassignedClaims           int
		expectedAssignedCDs                int
		expectedRunning                    int
		expectedLabels                     map[string]string // Tested on all clusters, so will not work if your test has pre-existing cds in the pool.
		// Map, keyed by claim name, of expected Status.Conditions['Pending'].Reason.
		// (The clusterpool controller always sets this condition's Status to True.)
		// Not checked if nil.
		expectedClaimPendingReasons map[string]string
		expectPoolVersionChanged    bool
	}{
		{
			name: "initialize conditions",
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithSize(1)),
			},
			expectedMissingDependenciesStatus: corev1.ConditionUnknown,
			expectedCapacityStatus:            corev1.ConditionUnknown,
			expectedCDCurrentStatus:           corev1.ConditionUnknown,
		},
		{
			name: "copyover fields",
			existing: []runtime.Object{
				// The test driver makes sure that "copyover fields" -- those we copy from the pool
				// spec to the CD spec -- match for CDs owned by the pool. This test case just
				// needs to a) set those fields in the pool spec, and b) have a nonzero size so the
				// pool actually creates CDs to compare.
				// TODO: Add coverage for more "copyover fields".
				initializedPoolBuilder.Build(testcp.WithSize(2), testcp.WithInstallAttemptsLimit(5)),
			},
			expectedTotalClusters: 2,
		},
		{
			name: "poolVersion changes with Platform",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithPlatform(hivev1.Platform{
					AWS: &aws.Platform{
						CredentialsSecretRef: corev1.LocalObjectReference{Name: "foo"},
					},
				})),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with BaseDomain",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithBaseDomain("foo.example.com")),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with ImageSet",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithImageSet("abc123")),
			},
			expectPoolVersionChanged: true,
		},
		{
			name: "poolVersion changes with InstallConfigSecretTemplate",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithInstallConfigSecretTemplateRef("abc123")),
			},
			expectPoolVersionChanged: true,
		},
		{
			// This also proves we only delete one stale cluster at a time
			name: "delete oldest stale cluster first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(
					testcd.WithPoolVersion("abc123"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("def345"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters: 1,
			// Note: these observed counts are calculated before we start adding/deleting
			// clusters, so they include the stale one we deleted.
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			expectedDeletedClusters: []string{"c2"},
		},
		{
			name: "delete stale unknown-version cluster first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				// Two CDs: One with an "unknown" poolVersion, the other older. Proving we favor the unknown for deletion.
				unclaimedCDBuilder("c1").Build(
					testcd.WithPoolVersion(""),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("bogus, but set"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			expectedDeletedClusters: []string{"c1"},
		},
		{
			name: "stale clusters not deleted if any CD still installing",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "stale clusters not deleted if under capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Installed(),
					testcd.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Hour))),
				),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "stale clusters not deleted if over capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").Build(
					testcd.Installed(),
				),
				unclaimedCDBuilder("c2").Build(
					testcd.WithPoolVersion("stale"),
					testcd.Running(),
				),
			},
			// This deletion happens because we're over capacity, not because we have staleness.
			// This is proven below.
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedObservedReady:   1,
			expectedCDCurrentStatus: corev1.ConditionFalse,
			// So this is kind of interesting. We delete c1 even though c2 is a) older, b) stale.
			// This is because we prioritize keeping running clusters, as they can satisfy claims
			// more quickly. Possible subject of a future optimization.
			expectedDeletedClusters: []string{"c1"},
		},
		{
			name: "delete broken unclaimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				// ProvisionStopped+Installed doesn't make sense IRL; but we're proving it would
				// be counted as broken if it did.
				unclaimedCDBuilder("c2").Build(testcd.Broken(), testcd.Installed()),
				// Unbroken CDs don't get deleted
				unclaimedCDBuilder("c3").Build(),
				// ...including assignable ones
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				// Claimed CDs don't get deleted
				cdBuilder("c5").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test1")),
				// ...even if they're broken
				cdBuilder("c6").Build(testcd.Broken(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test2")),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  4,
			// The broken one doesn't get counted as Ready
			expectedDeletedClusters: []string{"c1", "c2"},
			expectedAssignedCDs:     2,
		},
		{
			name: "deleting broken clusters is bounded by maxConcurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(testcd.Broken()),
				unclaimedCDBuilder("c3").Build(testcd.Broken()),
				unclaimedCDBuilder("c4").Build(testcd.Broken()),
			},
			expectedTotalClusters: 2,
			expectedObservedSize:  4,
			// Note: We can't expectDeletedClusters because we delete broken clusters in arbitrary order
		},
		{
			name: "adding takes precedence over deleting broken clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
		},
		{
			name: "delete broken clusters first when reducing capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				unclaimedCDBuilder("c1").Build(testcd.Broken()),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedTotalClusters:   1,
			expectedObservedSize:    2,
			expectedDeletedClusters: []string{"c1"},
		},
		{
			// HIVE-1684
			name: "broken clusters are deleted at MaxSize",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2), testcp.WithMaxSize(3)),
				unclaimedCDBuilder("c1").Build(
					testcd.Broken(),
				),
				cdBuilder("c2").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "aclaim")),
				cdBuilder("c3").Build(testcd.Installed(), testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "bclaim")),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    1,
			expectedObservedReady:   0,
			expectedAssignedCDs:     2,
			expectedDeletedClusters: []string{"c1"},
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
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 5,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with no more capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(3)),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
			expectedCapacityStatus: corev1.ConditionFalse,
		},
		{
			name: "scale up with some capacity including claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxSize(4)),
				testclaim.FullBuilder(testNamespace, "test", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.Installed(),
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with no more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with one more max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "scale up with max concurrent and max size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(5), testcp.WithMaxConcurrent(1)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  2,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "scale up with max concurrent and max size 3",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(6), testcp.WithMaxSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  4,
			expectedObservedSize:   3,
			expectedObservedReady:  1,
			expectedCapacityStatus: corev1.ConditionTrue,
		},
		{
			name: "no scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").GenericOptions(testgeneric.Deleted()).Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  2,
			// runningCount==0 and we don't have pending claims, but we don't muck with the
			// Running-ness of the deleting cluster.
			expectedRunning: 1,
		},
		{
			name: "no scale up with max concurrent and some deleting claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(2)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").GenericOptions(testgeneric.Deleted()).Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedAssignedClaims: 1,
			expectedAssignedCDs:    1,
		},
		{
			name: "scale up with max concurrent and some deleting",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(5), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
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
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "scale down with max concurrent enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "scale down with max concurrent not enough",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(testcd.Running()),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  6,
			expectedObservedReady: 3,
		},
		{
			name: "delete installing clusters first",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters:   2,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c3"},
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
			// Also shows we delete installed clusters before running ones
			name: "delete installed clusters when there are not enough installing to delete",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(testcd.Running()),
				unclaimedCDBuilder("c5").Build(testcd.Running()),
				unclaimedCDBuilder("c6").Build(),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    6,
			expectedObservedReady:   3,
			expectedDeletedClusters: []string{"c2", "c3", "c6"},
		},
		{
			name: "clusters deleted when clusterpool deleted",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.Deleted()).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
			},
			expectedTotalClusters:   0,
			expectFinalizerRemoved:  true,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},
		{
			name: "finalizer added to clusterpool",
			existing: []runtime.Object{
				initializedPoolBuilder.GenericOptions(testgeneric.WithoutFinalizer(finalizer)).Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedTotalClusters: 3,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "clusters not part of pool are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "claimed clusters are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
			expectedAssignedCDs:   1,
		},
		{
			name: "clusters in different pool are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, "other-pool", "test-claim"),
					testcd.WithPoolVersion("aversion")),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
		},
		{
			name: "deleting clusters are not counted against pool size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").GenericOptions(testgeneric.Deleted()).Build(testcd.Installed()),
				cdBuilder("c5").GenericOptions(testgeneric.Deleted()).Build(testcd.Running()),
				cdBuilder("c6").GenericOptions(testgeneric.Deleted()).Build(),
			},
			expectedTotalClusters: 6,
			expectedObservedSize:  3,
			expectedObservedReady: 1,
			expectedRunning:       1,
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
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
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
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
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
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
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
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").GenericOptions(testgeneric.Deleted()).Build(testcd.Running()),
			},
			expectedTotalClusters:  3,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
			expectedRunning:        1,
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
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Installed()),
			},
			expectedTotalClusters:  2,
			expectedObservedSize:   2,
			expectedObservedReady:  1,
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
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
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
			expectedCDCurrentStatus:            corev1.ConditionUnknown,
		},
		{
			name: "assign to claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:       4,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      1,
			expectedAssignedCDs:         1,
			expectedRunning:             1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ClusterAssigned"},
		},
		{
			name: "no ready clusters to assign to claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters:       4,
			expectedObservedSize:        3,
			expectedObservedReady:       0,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedRunning:             1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "NoClusters"},
		},
		{
			name: "assign to multiple claims",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(),
				// Claims are assigned in FIFO order by creationTimestamp
				testclaim.FullBuilder(testNamespace, "test-claim-1", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Second*2))),
				),
				testclaim.FullBuilder(testNamespace, "test-claim-2", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish.Add(-time.Second))),
				),
				testclaim.FullBuilder(testNamespace, "test-claim-3", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.Generic(testgeneric.WithCreationTimestamp(nowish)),
				),
			},
			expectedTotalClusters:    7,
			expectedObservedSize:     4,
			expectedObservedReady:    2,
			expectedAssignedClaims:   2,
			expectedAssignedCDs:      2,
			expectedRunning:          3,
			expectedUnassignedClaims: 1,
			expectedClaimPendingReasons: map[string]string{
				"test-claim-1": "ClusterAssigned",
				"test-claim-2": "ClusterAssigned",
				"test-claim-3": "NoClusters",
			},
		},
		{
			name: "do not assign to claims for other pools",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(
					testclaim.WithPool("other-pool"),
					testclaim.WithCondition(hivev1.ClusterClaimCondition{
						Type:    hivev1.ClusterClaimPendingCondition,
						Status:  corev1.ConditionFalse,
						Reason:  "ThisShouldNotChange",
						Message: "Claim ignored because not in the pool",
					}),
				),
			},
			expectedTotalClusters:       3,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ThisShouldNotChange"},
		},
		{
			name: "do not assign to claims in other namespaces",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				testclaim.FullBuilder("other-namespace", "test-claim", scheme).Build(
					testclaim.WithPool(testLeasePoolName),
					testclaim.WithCondition(hivev1.ClusterClaimCondition{
						Type:    hivev1.ClusterClaimPendingCondition,
						Status:  corev1.ConditionFalse,
						Reason:  "ThisShouldNotChange",
						Message: "Claim ignored because not in the namespace",
					}),
				),
			},
			expectedTotalClusters:       3,
			expectedObservedSize:        3,
			expectedObservedReady:       1,
			expectedAssignedClaims:      0,
			expectedUnassignedClaims:    1,
			expectedClaimPendingReasons: map[string]string{"test-claim": "ThisShouldNotChange"},
		},
		{
			name: "do not delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
				),
			},
			expectedTotalClusters: 4,
			expectedObservedSize:  3,
			expectedObservedReady: 2,
			expectedAssignedCDs:   1,
		},
		{
			name: "do not delete previously claimed clusters 2",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
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
			expectedAssignedCDs:   1,
		},
		{
			name: "delete previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(3)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
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
			expectedAssignedCDs:   1,
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
			expectedAssignedCDs:   1,
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
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale up should include previouly deleted in max concurrent all used by deleting previously claimed",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(4), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
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
			expectedObservedReady:   1,
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
			expectedAssignedCDs:     1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(),
				cdBuilder("c4").
					GenericOptions(testgeneric.WithAnnotation(constants.ClusterClaimRemoveClusterAnnotation, "true")).
					Build(
						testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					),
			},
			expectedTotalClusters:   3,
			expectedObservedSize:    3,
			expectedObservedReady:   1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by deleting previously claimed clusters",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
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
			expectedObservedReady:   2,
			expectedDeletedClusters: []string{"c4", "c5"},
		},
		{
			name: "scale down should include previouly deleted in max concurrent all used by one installing cluster and deleting one previously claimed cluster",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1), testcp.WithMaxConcurrent(2)),
				unclaimedCDBuilder("c1").Build(testcd.Installed()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
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
			expectedObservedReady:   1,
			expectedAssignedCDs:     1,
			expectedDeletedClusters: []string{"c4"},
		},
		{
			name: "claims exceed capacity",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters: 5,
			// The assignments don't happen until a subsequent reconcile after the CDs are ready
			expectedUnassignedClaims: 3,
			// Even though runningCount is zero, we run enough clusters to fulfill the excess claims
			expectedRunning: 3,
		},
		{
			name: "zero size pool",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(0)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedTotalClusters: 1,
			// The assignments don't happen until a subsequent reconcile after the CDs are ready
			expectedUnassignedClaims: 1,
			// Even though runningCount is zero, we run enough clusters to fulfill the excess claims
			expectedRunning: 1,
		},
		{
			name: "no CDs match pool version",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("a-previous-version")),
				cdBuilder("c2").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("some-other-version")),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "not all CDs match pool version",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("a-previous-version")),
				unclaimedCDBuilder("c2").Build(),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "empty pool version results in unknown CDCurrent status",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(1)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("")),
			},
			expectedObservedSize:    1,
			expectedTotalClusters:   1,
			expectedCDCurrentStatus: corev1.ConditionUnknown,
		},
		{
			name: "mismatched CDCurrent status takes precedence over unknown",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				cdBuilder("c1").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("")),
				cdBuilder("c2").Build(
					testcd.WithUnclaimedClusterPoolReference(testNamespace, testLeasePoolName),
					testcd.WithPoolVersion("some-other-version")),
			},
			expectedObservedSize:    2,
			expectedTotalClusters:   2,
			expectedCDCurrentStatus: corev1.ConditionFalse,
		},
		{
			name: "claimed CD at old version doesn't affect version status",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(testcp.WithSize(2)),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				cdBuilder("c1").Build(
					testcd.WithClusterPoolReference(testNamespace, testLeasePoolName, "test-claim"),
					testcd.WithPoolVersion("a-previous-version")),
				unclaimedCDBuilder("c2").Build(),
				unclaimedCDBuilder("c3").Build(),
			},
			expectedAssignedCDs:    1,
			expectedAssignedClaims: 1,
			expectedObservedSize:   2,
			expectedTotalClusters:  3,
		},
		{
			name: "runningCount < size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       2,
		},
		{
			name: "runningCount == size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(4),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       4,
		},
		{
			name: "runningCount > size",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(400),
				),
			},
			expectedTotalClusters: 4,
			expectedRunning:       4,
		},
		{
			name: "runningCount restored after deletion",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").Build(testcd.Generic(testgeneric.Deleted()), testcd.WithPowerState(hivev1.ClusterPowerStateRunning)),
				unclaimedCDBuilder("c2").Build(testcd.WithPowerState(hivev1.ClusterPowerStateRunning)),
				unclaimedCDBuilder("c3").Build(),
				unclaimedCDBuilder("c4").Build(),
			},
			expectedObservedSize:  3,
			expectedTotalClusters: 5,
			expectedRunning:       3,
		},
		{
			name: "runningCount restored after claim",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:   4,
			expectedObservedReady:  2,
			expectedTotalClusters:  5,
			expectedRunning:        3,
			expectedAssignedCDs:    1,
			expectedAssignedClaims: 1,
		},
		{
			name: "pool size > number of claims > runningCount",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(3),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Running()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:   4,
			expectedObservedReady:  3,
			expectedTotalClusters:  7,
			expectedRunning:        6,
			expectedAssignedCDs:    3,
			expectedAssignedClaims: 3,
		},
		{
			name: "number of claims > pool size > runningCount",
			existing: []runtime.Object{
				initializedPoolBuilder.Build(
					testcp.WithSize(4),
					testcp.WithRunningCount(2),
				),
				unclaimedCDBuilder("c1").Build(testcd.Running()),
				unclaimedCDBuilder("c2").Build(testcd.Running()),
				unclaimedCDBuilder("c3").Build(testcd.Installed()),
				unclaimedCDBuilder("c4").Build(testcd.Installed()),
				testclaim.FullBuilder(testNamespace, "test-claim1", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim2", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim3", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim4", scheme).Build(testclaim.WithPool(testLeasePoolName)),
				testclaim.FullBuilder(testNamespace, "test-claim5", scheme).Build(testclaim.WithPool(testLeasePoolName)),
			},
			expectedObservedSize:  4,
			expectedObservedReady: 2,
			expectedTotalClusters: 9,
			// The two original running pool CDs got claimed.
			// We create five new CDs to satisfy the pool size plus the additional three claims.
			// Of those, we start two for runningCount, and three more for the excess claims.
			// Including the two originally running that we assigned to claims, that's seven.
			expectedRunning:          7,
			expectedAssignedCDs:      2,
			expectedAssignedClaims:   2,
			expectedUnassignedClaims: 3,
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
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(test.existing...).Build()
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

			pool := &hivev1.ClusterPool{}
			err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testLeasePoolName}, pool)

			if test.expectFinalizerRemoved {
				assert.True(t, apierrors.IsNotFound(err), "expected pool to be deleted")
			} else {
				assert.NoError(t, err, "unexpected error getting clusterpool")
				assert.Contains(t, pool.Finalizers, finalizer, "expect finalizer on clusterpool")
				assert.Equal(t, test.expectedObservedSize, pool.Status.Size, "unexpected observed size")
				assert.Equal(t, test.expectedObservedReady, pool.Status.Ready, "unexpected observed ready count")
				currentPoolVersion := calculatePoolVersion(pool)
				assert.Equal(
					t, test.expectPoolVersionChanged, currentPoolVersion != initialPoolVersion,
					"expectPoolVersionChanged is %t\ninitial %q\nfinal   %q",
					test.expectPoolVersionChanged, initialPoolVersion, currentPoolVersion)
				expectedCDCurrentStatus := test.expectedCDCurrentStatus
				if expectedCDCurrentStatus == "" {
					expectedCDCurrentStatus = corev1.ConditionTrue
				}
				cdCurrentCondition := controllerutils.FindClusterPoolCondition(pool.Status.Conditions, hivev1.ClusterPoolAllClustersCurrentCondition)
				if assert.NotNil(t, cdCurrentCondition, "did not find ClusterDeploymentsCurrent condition") {
					assert.Equal(t, expectedCDCurrentStatus, cdCurrentCondition.Status,
						"unexpected ClusterDeploymentsCurrent condition")
				}
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

			cds := &hivev1.ClusterDeploymentList{}
			err = fakeClient.List(context.Background(), cds)
			require.NoError(t, err)

			assert.Len(t, cds.Items, test.expectedTotalClusters, "unexpected number of total clusters")

			for _, expectedDeletedName := range test.expectedDeletedClusters {
				for _, cd := range cds.Items {
					assert.NotEqual(t, expectedDeletedName, cd.Name, "expected cluster to have been deleted")
				}
			}

			var actualAssignedCDs, actualUnassignedCDs, actualRunning, actualHibernating int
			for _, cd := range cds.Items {
				poolRef := cd.Spec.ClusterPoolRef
				if poolRef == nil || poolRef.PoolName != testLeasePoolName || poolRef.ClaimName == "" {
					// TODO: Calling these "unassigned" isn't great. Some may not even belong to the pool.
					actualUnassignedCDs++
				} else {
					actualAssignedCDs++
				}
				// Match up copyover fields for any clusters belonging to the pool
				if poolRef != nil && poolRef.PoolName == testLeasePoolName {
					// TODO: These would need to be set on all CDs in test.existing
					// assert.Equal(t, pool.Spec.BaseDomain, cd.Spec.BaseDomain, "expected BaseDomain to match")
					if pool.Spec.InstallAttemptsLimit != nil {
						if assert.NotNil(t, cd.Spec.InstallAttemptsLimit, "expected InstallAttemptsLimit to be set") {
							assert.Equal(t, *pool.Spec.InstallAttemptsLimit, *cd.Spec.InstallAttemptsLimit, "expected InstallAttemptsLimit to match")
						}
					}
				}
				switch powerState := cd.Spec.PowerState; powerState {
				case hivev1.ClusterPowerStateRunning:
					actualRunning++
				case hivev1.ClusterPowerStateHibernating:
					actualHibernating++
				}

				if test.expectedLabels != nil {
					for k, v := range test.expectedLabels {
						assert.Equal(t, v, cd.Labels[k])
					}
				}
			}
			assert.Equal(t, test.expectedAssignedCDs, actualAssignedCDs, "unexpected number of assigned CDs")
			assert.Equal(t, test.expectedTotalClusters-test.expectedAssignedCDs, actualUnassignedCDs, "unexpected number of unassigned CDs")
			assert.Equal(t, test.expectedRunning, actualRunning, "unexpected number of running CDs")
			assert.Equal(t, test.expectedTotalClusters-test.expectedRunning, actualHibernating, "unexpected number of hibernating CDs")

			claims := &hivev1.ClusterClaimList{}
			err = fakeClient.List(context.Background(), claims)
			require.NoError(t, err)

			actualAssignedClaims := 0
			actualUnassignedClaims := 0
			for _, claim := range claims.Items {
				if test.expectedClaimPendingReasons != nil {
					if reason, ok := test.expectedClaimPendingReasons[claim.Name]; ok {
						actualCond := controllerutils.FindClusterClaimCondition(claim.Status.Conditions, hivev1.ClusterClaimPendingCondition)
						if assert.NotNil(t, actualCond, "did not find Pending condition on claim %s", claim.Name) {
							assert.Equal(t, reason, actualCond.Reason, "wrong reason on Pending condition for claim %s", claim.Name)
						}
					}
				}
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
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(test.existing...).Build()
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

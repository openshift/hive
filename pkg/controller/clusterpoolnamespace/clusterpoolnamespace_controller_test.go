package clusterpoolnamespace

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcp "github.com/openshift/hive/pkg/test/clusterpool"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testnamespace "github.com/openshift/hive/pkg/test/namespace"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	namespaceName = "test-namespace"
	cdName        = "test-cluster-deployment"
	crName        = "test-cluster-relocator"
	testFinalizer = "test-finalizer"
)

func TestReconcileClusterPoolNamespace_Reconcile_Movement(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := scheme.GetScheme()

	namespaceWithoutLabelBuilder := testnamespace.FullBuilder(namespaceName, scheme)
	namespaceBuilder := namespaceWithoutLabelBuilder.GenericOptions(
		testgeneric.WithLabel(constants.ClusterPoolNameLabel, "test-cluster-pool"),
	)

	poolBuilder := testcp.FullBuilder("test-pool-namespace", "test-cluster-pool", scheme).
		Options(
			testcp.ForAWS("aws-creds", "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet("test-image-set"),
		)

	validateNoRequeueAfter := func(t *testing.T, requeueAfter time.Duration, startTime, endTime time.Time) {
		assert.Zero(t, requeueAfter, "unexpected non-zero requeue after")
	}
	validateWaitForCDGoneRequeueAfter := func(t *testing.T, requeueAfter time.Duration, startTime, endTime time.Time) {
		assert.Equal(t, durationBetweenDeletingClusterDeploymentChecks, requeueAfter, "expected requeue after for checking deleting ClusterDeployments")
	}

	cases := []struct {
		name                 string
		namespaceBuilder     testnamespace.Builder
		extraNamespaceOption func(startTime time.Time) testnamespace.Option
		resources            []runtime.Object
		expectDeleted        bool
		validateRequeueAfter func(t *testing.T, requeueAfter time.Duration, startTime, endTime time.Time)
	}{
		{
			name:                 "no clusterdeployments",
			namespaceBuilder:     namespaceBuilder,
			expectDeleted:        true,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:             "non-deleted clusterdeployment",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd", scheme).Build(),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:             "non-deleted clusterdeployment with existing pool",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				poolBuilder.Build(),
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim")),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:             "non-deleted clusterdeployment with existing pool 2",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				poolBuilder.Build(),
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim")),
				testcd.FullBuilder(namespaceName, "test-cd-2", scheme).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim-2")),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:             "non-deleted clusterdeployment with no pool",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim")),
				testcd.FullBuilder(namespaceName, "test-cd-2", scheme).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim-2")),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateWaitForCDGoneRequeueAfter,
		},
		{
			name:             "deleted clusterdeployment",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).
					Build(),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateWaitForCDGoneRequeueAfter,
		},
		{
			name:             "deleted clusterdeployment with no pool",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true"), testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim")),
				testcd.FullBuilder(namespaceName, "test-cd-2", scheme).
					GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).
					Build(testcd.WithClusterPoolReference("test-pool-namespace", "test-cluster-pool", "test-claim-2")),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateWaitForCDGoneRequeueAfter,
		},
		{
			name:             "deleted and non-deleted clusterdeployments",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd-1", scheme).Build(),
				testcd.FullBuilder(namespaceName, "test-cd-2", scheme).
					GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).
					Build(),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:                 "deleted namespace",
			namespaceBuilder:     namespaceBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)),
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:                 "namespace without clusterpool label",
			namespaceBuilder:     namespaceWithoutLabelBuilder,
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:             "namespace too young",
			namespaceBuilder: namespaceBuilder,
			extraNamespaceOption: func(startTime time.Time) testnamespace.Option {
				return func(namespace *corev1.Namespace) {
					namespace.CreationTimestamp.Time = startTime.Add(-1 * time.Minute)
				}
			},
			expectDeleted: false,
			validateRequeueAfter: func(t *testing.T, requeueAfter time.Duration, startTime, endTime time.Time) {
				// CreationTimestamp only has granularity to the second, so add a 1-second buffer on each end of the limits.
				upperBound := minimumLifetime - 1*time.Minute + 1*time.Second
				lowerBound := minimumLifetime - 1*time.Minute - endTime.Sub(startTime) - 1*time.Second
				assert.LessOrEqual(t, lowerBound.Seconds(), requeueAfter.Seconds(), "requeue after too small for waiting for ClusterDeployment creation")
				assert.GreaterOrEqual(t, upperBound.Seconds(), requeueAfter.Seconds(), "requeue after too large for waiting for ClusterDeployment creation")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			startTime := time.Now()
			if tc.namespaceBuilder != nil {
				builder := tc.namespaceBuilder
				if tc.extraNamespaceOption != nil {
					builder = builder.Options(tc.extraNamespaceOption(startTime))
				}
				tc.resources = append(tc.resources, builder.Build())
			}
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.resources...).Build()

			reconciler := &ReconcileClusterPoolNamespace{
				Client: c,
				logger: logger,
			}
			namespaceKey := client.ObjectKey{Name: namespaceName}
			result, err := reconciler.Reconcile(context.TODO(), reconcile.Request{NamespacedName: namespaceKey})
			endTime := time.Now()
			require.NoError(t, err, "unexpected error during reconcile")

			namespace := &corev1.Namespace{}
			err = c.Get(context.Background(), namespaceKey, namespace)
			assert.Equal(t, tc.expectDeleted, apierrors.IsNotFound(err), "unexpected state of namespace existence")

			tc.validateRequeueAfter(t, result.RequeueAfter, startTime, endTime)
		})
	}
}

func Test_cleanupPreviouslyClaimedClusterDeployments(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)
	scheme := scheme.GetScheme()

	poolBuilder := testcp.FullBuilder("test-namespace", "test-cluster-pool", scheme).
		Options(
			testcp.ForAWS("aws-creds", "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet("test-image-set"),
		)
	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme).Options(
			testcd.WithPowerState(hivev1.ClusterPowerStateHibernating),
		)
	}
	cdBuilderWithPool := func(name, pool string) testcd.Builder {
		return cdBuilder(name).Options(testcd.WithClusterPoolReference("test-namespace", pool, "test-claim"))
	}

	cases := []struct {
		name             string
		resources        []runtime.Object
		expectedErr      string
		expectedCleanup  bool
		expectedClusters int
	}{{
		name: "no cleanup as clusterdeployment not for pool",
		resources: []runtime.Object{
			cdBuilder("cd1").Build(),
			cdBuilder("cd2").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "no cleanup as cluster pool exists",
		resources: []runtime.Object{
			poolBuilder.Build(),
			cdBuilderWithPool("cd1", "test-cluster-pool").Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "no cleanup as cluster pool exists with marked for removal clusters",
		resources: []runtime.Object{
			poolBuilder.Build(),
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "no cleanup as all clusters already deleted",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "no cleanup as no clusters marked for removal",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "no cleanup as clusters marked for removal already deleted",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer), testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  false,
		expectedClusters: 2,
	}, {
		name: "some cleanup",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
		},
		expectedErr:      "",
		expectedCleanup:  true,
		expectedClusters: 1,
	}, {
		name: "some cleanup 2",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer), testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
		},
		expectedErr:      "",
		expectedCleanup:  true,
		expectedClusters: 1,
	}, {
		name: "some cleanup 3",
		resources: []runtime.Object{
			cdBuilderWithPool("cd1", "test-cluster-pool").GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd2", "test-cluster-pool").Build(),
			cdBuilderWithPool("cd3", "test-cluster-pool").GenericOptions(testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
			cdBuilderWithPool("cd4", "test-cluster-pool").GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer), testgeneric.WithAnnotation(constants.RemovePoolClusterAnnotation, "true")).Build(),
		},
		expectedErr:      "",
		expectedCleanup:  true,
		expectedClusters: 2,
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.resources...).Build()
			reconciler := &ReconcileClusterPoolNamespace{
				Client: c,
				logger: logger,
			}
			cds := &hivev1.ClusterDeploymentList{}
			err := c.List(context.Background(), cds)
			require.NoError(t, err)

			cleanup, err := reconciler.cleanupPreviouslyClaimedClusterDeployments(cds, logger)
			if test.expectedErr != "" {
				assert.Regexp(t, test.expectedErr, err)
			}
			assert.Equal(t, test.expectedCleanup, cleanup)

			cds = &hivev1.ClusterDeploymentList{}
			err = c.List(context.Background(), cds)
			require.NoError(t, err)

			assert.Len(t, cds.Items, test.expectedClusters)
		})
	}
}

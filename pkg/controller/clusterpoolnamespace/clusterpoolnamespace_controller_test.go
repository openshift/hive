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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	testnamespace "github.com/openshift/hive/pkg/test/namespace"
)

const (
	namespaceName = "test-namespace"
	cdName        = "test-cluster-deployment"
	crName        = "test-cluster-relocator"
)

func TestReconcileClusterPoolNamespace_Reconcile_Movement(t *testing.T) {
	logger := log.New()
	logger.SetLevel(log.DebugLevel)

	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	hivev1.AddToScheme(scheme)

	namespaceWithoutLabelBuilder := testnamespace.FullBuilder(namespaceName, scheme)
	namespaceBuilder := namespaceWithoutLabelBuilder.GenericOptions(
		testgeneric.WithLabel(constants.ClusterPoolNameLabel, "test-cluster-pool"),
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
			name:             "deleted clusterdeployment",
			namespaceBuilder: namespaceBuilder,
			resources: []runtime.Object{
				testcd.FullBuilder(namespaceName, "test-cd", scheme).
					GenericOptions(testgeneric.Deleted()).
					Build(),
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
					GenericOptions(testgeneric.Deleted()).
					Build(),
			},
			expectDeleted:        false,
			validateRequeueAfter: validateNoRequeueAfter,
		},
		{
			name:                 "deleted namespace",
			namespaceBuilder:     namespaceBuilder.GenericOptions(testgeneric.Deleted()),
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
			c := fake.NewFakeClientWithScheme(scheme, tc.resources...)

			reconciler := &ReconcileClusterPoolNamespace{
				Client: c,
				logger: logger,
			}
			namespaceKey := client.ObjectKey{Name: namespaceName}
			result, err := reconciler.Reconcile(reconcile.Request{NamespacedName: namespaceKey})
			endTime := time.Now()
			require.NoError(t, err, "unexpected error during reconcile")

			namespace := &corev1.Namespace{}
			err = c.Get(context.Background(), namespaceKey, namespace)
			assert.Equal(t, tc.expectDeleted, apierrors.IsNotFound(err), "unexpected state of namespace existence")

			tc.validateRequeueAfter(t, result.RequeueAfter, startTime, endTime)
		})
	}
}

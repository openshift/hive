package clusterclaim

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

const (
	claimNamespace = "claim-namespace"
	claimName      = "test-claim"
)

func TestReconcileClusterClaim(t *testing.T) {
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)

	claimBuilder := testclaim.FullBuilder(claimNamespace, claimName, scheme)
	cdBuilder := func(name string) testcd.Builder {
		return testcd.FullBuilder(name, name, scheme)
	}

	tests := []struct {
		name                                   string
		existing                               []runtime.Object
		expectCompletedClaim                   bool
		expectNoAssignment                     bool
		expectedConditions                     []hivev1.ClusterClaimCondition
		expectNoFinalizer                      bool
		expectAssignedClusterDeploymentDeleted bool
	}{
		{
			name: "new assignment",
			existing: []runtime.Object{
				claimBuilder.Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			},
			expectCompletedClaim: true,
		},
		{
			name: "existing assignment",
			existing: []runtime.Object{
				claimBuilder.Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "assignment conflict",
			existing: []runtime.Object{
				claimBuilder.Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			},
			expectNoAssignment: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimPendingCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "AssignmentConflict",
				Message: "Assigned cluster was claimed by a different ClusterClaim",
			}},
		},
		{
			name: "deleted cluster",
			existing: []runtime.Object{
				claimBuilder.Build(testclaim.WithCluster("test-cluster")),
			},
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimClusterDeletedCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "ClusterDeleted",
				Message: "Assigned cluster has been deleted",
			}},
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "new assignment clears pending condition",
			existing: []runtime.Object{
				claimBuilder.Build(
					testclaim.WithCluster("test-cluster"),
					testclaim.WithCondition(
						hivev1.ClusterClaimCondition{
							Type:   hivev1.ClusterClaimPendingCondition,
							Status: corev1.ConditionTrue,
						},
					),
				),
				cdBuilder("test-cluster").Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			},
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimPendingCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterClaimed",
				Message: "Cluster claimed",
			}},
		},
		{
			name: "deleted claim with no assignment",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(),
			},
			expectNoAssignment:                     true,
			expectNoFinalizer:                      true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with unclaimed assignment",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			},
			expectNoFinalizer: true,
		},
		{
			name: "deleted claim with claimed assignment",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			},
			expectCompletedClaim: true,
			expectNoFinalizer:    true,
		},
		{
			name: "deleted claim with missing clusterdeployment",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(testclaim.WithCluster("test-cluster")),
			},
			expectNoFinalizer:                      true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with deleted clusterdeployment",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").
					GenericOptions(testgeneric.Deleted()).
					Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			},
			expectCompletedClaim: true,
			expectNoFinalizer:    true,
		},
		{
			name: "deleted claim with clusterdeployment assigned to other claim",
			existing: []runtime.Object{
				claimBuilder.GenericOptions(testgeneric.Deleted()).Build(testclaim.WithCluster("test-cluster")),
				cdBuilder("test-cluster").Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			},
			expectNoFinalizer: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := fake.NewFakeClientWithScheme(scheme, test.existing...)
			logger := log.New()
			logger.SetLevel(log.DebugLevel)
			rcp := &ReconcileClusterClaim{
				Client: c,
				logger: logger,
			}

			reconcileRequest := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      claimName,
					Namespace: claimNamespace,
				},
			}

			_, err := rcp.Reconcile(reconcileRequest)
			require.NoError(t, err, "unexpected error from Reconcile")

			claim := &hivev1.ClusterClaim{}
			err = c.Get(context.Background(), client.ObjectKey{Namespace: claimNamespace, Name: claimName}, claim)
			require.NoError(t, err, "unexpected error getting claim")

			cds := &hivev1.ClusterDeploymentList{}
			err = c.List(context.Background(), cds)
			require.NoError(t, err, "unexpected error getting cluster deployments")

			assignedClusterDeploymentExists := false
			for _, cd := range cds.Items {
				isAssignedCD := cd.Name == claim.Spec.Namespace
				if isAssignedCD && test.expectCompletedClaim {
					assert.Equal(t, claimName, cd.Spec.ClusterPoolRef.ClaimName, "expected ClusterDeployment to be claimed by ClusterClaim")
				} else {
					assert.NotEqual(t, claimName, cd.Spec.ClusterPoolRef.ClaimName, "expected ClusterDeployment to not be claimed by ClusterClaim")
				}
				if isAssignedCD {
					assignedClusterDeploymentExists = true
				}
			}
			if claim.Spec.Namespace != "" {
				assert.NotEqual(t, test.expectAssignedClusterDeploymentDeleted, assignedClusterDeploymentExists, "unexpected assigned ClusterDeployment")
			}

			if test.expectNoAssignment {
				assert.Empty(t, claim.Spec.Namespace, "expected no assignment set on claim")
			} else {
				assert.NotEmpty(t, claim.Spec.Namespace, "expected assignment set on claim")
			}

			for i := range claim.Status.Conditions {
				cond := &claim.Status.Conditions[i]
				cond.LastTransitionTime = metav1.Time{}
				cond.LastProbeTime = metav1.Time{}
			}
			assert.ElementsMatch(t, test.expectedConditions, claim.Status.Conditions, "unexpected conditions")

			if test.expectNoFinalizer {
				assert.NotContains(t, claim.Finalizers, finalizer, "expected no finalizer on claim")
			} else {
				assert.Contains(t, claim.Finalizers, finalizer, "expected finalizer on claim")
			}
		})
	}
}

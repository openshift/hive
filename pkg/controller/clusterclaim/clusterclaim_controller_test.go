package clusterclaim

import (
	"context"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testcp "github.com/openshift/hive/pkg/test/clusterpool"
	testfake "github.com/openshift/hive/pkg/test/fake"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	claimNamespace       = "claim-namespace"
	claimName            = "test-claim"
	clusterName          = "test-cluster"
	kubeconfigSecretName = "kubeconfig-secret"
	passwordSecretName   = "password-secret"
	testLeasePoolName    = "test-cluster-pool"
	testFinalizer        = "test-finalizer"
)

func init() {
	log.SetLevel(log.DebugLevel)
}

var (
	subjects = []rbacv1.Subject{
		{
			APIGroup: rbacv1.GroupName,
			Kind:     rbacv1.GroupKind,
			Name:     "test-group",
		},
		{
			APIGroup: rbacv1.GroupName,
			Kind:     rbacv1.UserKind,
			Name:     "test-user",
		},
	}
)

func TestReconcileClusterClaim(t *testing.T) {
	scheme := scheme.GetScheme()
	poolBuilder := testcp.FullBuilder(claimNamespace, testLeasePoolName, scheme).
		GenericOptions(
			testgeneric.WithFinalizer(finalizer),
		).
		Options(
			testcp.ForAWS("secret", "us-east-1"),
			testcp.WithBaseDomain("test-domain"),
			testcp.WithImageSet("test-imageset"),
		)
	claimBuilder := testclaim.FullBuilder(claimNamespace, claimName, scheme).Options(
		testclaim.WithSubjects(subjects),
	)
	initializedClaimBuilder := testclaim.FullBuilder(claimNamespace, claimName, scheme).Options(
		testclaim.WithSubjects(subjects),
		testclaim.WithCondition(hivev1.ClusterClaimCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterClaimPendingCondition,
		}),
		testclaim.WithCondition(hivev1.ClusterClaimCondition{
			Status: corev1.ConditionUnknown,
			Type:   hivev1.ClusterRunningCondition,
		}),
	)
	cdBuilder := testcd.FullBuilder(clusterName, clusterName, scheme).Options(
		func(cd *hivev1.ClusterDeployment) {
			cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: kubeconfigSecretName},
				AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: passwordSecretName},
			}
		},
	)

	tests := []struct {
		name                                   string
		claim                                  *hivev1.ClusterClaim
		cd                                     *hivev1.ClusterDeployment
		existing                               []runtime.Object
		expectCompletedClaim                   bool
		expectNoAssignment                     bool
		expectedConditions                     []hivev1.ClusterClaimCondition
		expectNoFinalizer                      bool
		expectAssignedClusterDeploymentDeleted bool
		expectRBAC                             bool
		expectHibernating                      bool
		expectDeleted                          bool
		expectedRequeueAfter                   *time.Duration
	}{
		{
			name:  "initialize conditions",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			expectNoFinalizer: true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:   hivev1.ClusterClaimPendingCondition,
					Status: corev1.ConditionUnknown,
				},
				{
					Type:   hivev1.ClusterRunningCondition,
					Status: corev1.ConditionUnknown,
				},
			},
		},
		{
			name:  "unassigned CD is a no-op",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool"),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:   hivev1.ClusterClaimPendingCondition,
					Status: corev1.ConditionUnknown,
				},
				{
					Type:   hivev1.ClusterRunningCondition,
					Status: corev1.ConditionUnknown,
				},
			},
		},
		{
			name:  "existing assignment",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
		},
		{
			name:  "existing assignment running",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateRunning),
			),
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionTrue,
					Reason:  "Running",
					Message: "Cluster is running",
				},
			},
		},
		{
			name:               "assignment conflict",
			claim:              initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:                 cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			expectNoAssignment: true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:   hivev1.ClusterClaimPendingCondition,
					Status: corev1.ConditionUnknown,
				},
				{
					Type:   hivev1.ClusterRunningCondition,
					Status: corev1.ConditionUnknown,
				}},
		},
		{
			name:  "deleting cluster",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
			),
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterRunningCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterDeleting",
				Message: "Assigned cluster has been marked for deletion",
			}},
		},
		{
			name:  "deleted cluster",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterRunningCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterDeleted",
				Message: "Assigned cluster has been deleted",
			}},
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "existing assignment clears pending condition",
			claim: initializedClaimBuilder.Build(
				testclaim.WithCluster(clusterName),
				testclaim.WithCondition(
					hivev1.ClusterClaimCondition{
						Type:   hivev1.ClusterClaimPendingCondition,
						Status: corev1.ConditionTrue,
					},
				),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
			expectRBAC: true,
		},
		{
			name: "deleted claim with no assignment",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(),
			expectNoAssignment:                     true,
			expectDeleted:                          true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with unclaimed assignment",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectDeleted: true,
			expectRBAC:    true,
		},
		{
			name: "deleted claim with claimed assignment",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim:                   true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with missing clusterdeployment, role, and rolebinding",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			expectDeleted:                          true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with deleted clusterdeployment",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.GenericOptions(testgeneric.Deleted(), testgeneric.WithFinalizer(testFinalizer)).
				Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim: true,
		},
		{
			name: "deleted claim with clusterdeployment assigned to other claim",
			claim: initializedClaimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd:            cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			expectDeleted: true,
		},
		{
			name:  "no RBAC when no subjects",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName), testclaim.WithSubjects(nil)),
			cd: cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
		},
		{
			name:  "update existing role",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				func() runtime.Object {
					r := testRole()
					r.Rules = nil
					return r
				}(),
			},
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
		},
		{
			name:  "update existing rolebinding",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				func() runtime.Object {
					rb := testRoleBinding()
					rb.Subjects = nil
					rb.RoleRef = rbacv1.RoleRef{}
					return rb
				}(),
			},
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
		},
		{
			name:  "existing assignment does not change power state",
			claim: initializedClaimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithPowerState(hivev1.ClusterPowerStateHibernating),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectHibernating:    true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
		},
		{
			name: "claim with elapsed lifetime is deleted",
			claim: initializedClaimBuilder.Build(
				testclaim.WithCluster(clusterName),
				testclaim.WithLifetime(1*time.Hour),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			expectCompletedClaim: true,
		},
		{
			name: "claim with elapsed pool lifetime is deleted as set by pool default",
			claim: initializedClaimBuilder.Build(
				testclaim.WithPool(testLeasePoolName),
				testclaim.WithCluster(clusterName),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithDefaultClaimLifetime(1 * time.Hour)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "claim with elapsed pool lifetime is deleted as set by pool maximum",
			claim: initializedClaimBuilder.Build(
				testclaim.WithPool(testLeasePoolName),
				testclaim.WithCluster(clusterName),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithMaximumClaimLifetime(1 * time.Hour)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "claim with elapsed smaller lifetime is deleted, where pool maximum is smaller",
			claim: initializedClaimBuilder.Build(
				testclaim.WithPool(testLeasePoolName),
				testclaim.WithCluster(clusterName),
				testclaim.WithLifetime(2*time.Hour),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithMaximumClaimLifetime(1 * time.Hour)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "claim with elapsed smaller lifetime is deleted, where pool maximum is smaller than default",
			claim: initializedClaimBuilder.Build(
				testclaim.WithPool(testLeasePoolName),
				testclaim.WithCluster(clusterName),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithDefaultClaimLifetime(2*time.Hour), testcp.WithMaximumClaimLifetime(1*time.Hour)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "claim with elapsed smaller lifetime is deleted, where claim is smaller",
			claim: initializedClaimBuilder.Build(
				testclaim.WithPool(testLeasePoolName),
				testclaim.WithCluster(clusterName),
				testclaim.WithLifetime(1*time.Hour),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				poolBuilder.Build(testcp.WithMaximumClaimLifetime(2 * time.Hour)),
			},
			expectCompletedClaim: true,
		},
		{
			name: "claim with non-elapsed lifetime is not deleted",
			claim: initializedClaimBuilder.Build(
				testclaim.WithCluster(clusterName),
				testclaim.WithLifetime(3*time.Hour),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					Reason:             "ClusterClaimed",
					Message:            "Cluster claimed",
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithStatusPowerState(hivev1.ClusterPowerStateStartingMachines),
			),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{
				{
					Type:    hivev1.ClusterClaimPendingCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "ClusterClaimed",
					Message: "Cluster claimed",
				},
				{
					Type:    hivev1.ClusterRunningCondition,
					Status:  corev1.ConditionFalse,
					Reason:  "Resuming",
					Message: "Waiting for cluster to be running",
				},
			},
			expectRBAC:           true,
			expectedRequeueAfter: func(d time.Duration) *time.Duration { return &d }(2 * time.Hour),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.claim != nil {
				test.existing = append(test.existing, test.claim)
			}
			if test.cd != nil {
				test.existing = append(test.existing, test.cd)
			}
			c := testfake.NewFakeClientBuilder().WithRuntimeObjects(test.existing...).Build()
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

			result, err := rcp.Reconcile(context.TODO(), reconcileRequest)
			require.NoError(t, err, "unexpected error from Reconcile")

			if test.expectedRequeueAfter == nil {
				assert.Zero(t, result.RequeueAfter, "expected no requeue after")
			} else {
				assert.GreaterOrEqual(t, result.RequeueAfter.Seconds(), (*test.expectedRequeueAfter - 10*time.Second).Seconds(), "requeue after too small")
				assert.LessOrEqual(t, result.RequeueAfter.Seconds(), (*test.expectedRequeueAfter + 10*time.Second).Seconds(), "requeue after too large")
			}

			claim := &hivev1.ClusterClaim{}
			err = c.Get(context.Background(), client.ObjectKey{Namespace: claimNamespace, Name: claimName}, claim)
			if !test.expectDeleted {
				require.NoError(t, err, "unexpected error getting claim")
				if test.expectNoFinalizer {
					assert.NotContains(t, claim.Finalizers, finalizer, "expected no finalizer on claim")
				} else {
					assert.Contains(t, claim.Finalizers, finalizer, "expected finalizer on claim")
				}
			} else {
				assert.True(t, apierrors.IsNotFound(err), "expected claim to be deleted")
			}

			// If the claim was deleted, we still need to be able to check things against the
			// fields that existed in it before the Reconcile
			if test.expectDeleted {
				claim = test.claim
			}

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
					toRemove := controllerutils.IsClusterMarkedForRemoval(&cd)
					assignedClusterDeploymentExists = !toRemove
					if test.expectHibernating {
						assert.Equal(t, hivev1.ClusterPowerStateHibernating, cd.Spec.PowerState, "expected ClusterDeployment to be hibernating")
					} else {
						assert.NotEqual(t, hivev1.ClusterPowerStateHibernating, cd.Spec.PowerState, "expected ClusterDeployment to not be hibernating")
					}
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

			if test.expectDeleted && len(test.expectedConditions) != 0 {
				t.Fatal("test configuration error: Can't expect deleted claim and also expect conditions")
			}

			for _, expectedCond := range test.expectedConditions {
				cond := controllerutils.FindCondition(claim.Status.Conditions, expectedCond.Type)
				if assert.NotNilf(t, cond, "did not find expected condition type: %v", expectedCond.Type) {
					assert.Equal(t, expectedCond.Status, cond.Status, "condition found with unexpected status")
					if expectedCond.Reason != "" {
						assert.Equal(t, expectedCond.Reason, cond.Reason, "condition found with unexpected reason")
					}
					if expectedCond.Message != "" {
						assert.Equal(t, expectedCond.Message, cond.Message, "condition found with unexpected message")
					}
				}
			}

			role := &rbacv1.Role{}
			getRoleError := c.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: hiveClaimOwnerRoleName}, role)
			roleBinding := &rbacv1.RoleBinding{}
			getRoleBindingError := c.Get(context.Background(), client.ObjectKey{Namespace: clusterName, Name: hiveClaimOwnerRoleBindingName}, roleBinding)
			if test.expectRBAC {
				assert.NoError(t, getRoleError, "unexpected error getting role")
				assert.NoError(t, getRoleBindingError, "unexpected error getting role binding")
				expectedRules := []rbacv1.PolicyRule{
					{
						APIGroups: []string{"hive.openshift.io"},
						Resources: []string{"*"},
						Verbs:     []string{"*"},
					},
					{
						APIGroups:     []string{""},
						Resources:     []string{"secrets"},
						ResourceNames: []string{kubeconfigSecretName, passwordSecretName},
						Verbs:         []string{"get"},
					},
				}
				assert.Equal(t, expectedRules, role.Rules, "unexpected role rules")
				assert.Equal(t, subjects, roleBinding.Subjects, "unexpected role binding subjects")
				expectedRoleRef := rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "Role",
					Name:     hiveClaimOwnerRoleName,
				}
				assert.Equal(t, expectedRoleRef, roleBinding.RoleRef, "unexpected role binding role ref")
			} else {
				assert.True(t, apierrors.IsNotFound(getRoleError), "expected no role")
				assert.True(t, apierrors.IsNotFound(getRoleBindingError), "expected no role binding")
			}
		})
	}
}

func Test_getClaimLifetime(t *testing.T) {
	cases := []struct {
		name string

		defaultLifetime *metav1.Duration
		maximumLifetime *metav1.Duration
		claimLifetime   *metav1.Duration

		expected *metav1.Duration
	}{{
		name: "no lifetime set",

		expected: nil,
	}, {
		name:          "claim lifetime set",
		claimLifetime: &metav1.Duration{Duration: 1 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "no claim lifetime set, but default for pool",
		defaultLifetime: &metav1.Duration{Duration: 1 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "claim lifetime set, and default for pool",
		defaultLifetime: &metav1.Duration{Duration: 1 * time.Hour},
		claimLifetime:   &metav1.Duration{Duration: 3 * time.Hour},

		expected: &metav1.Duration{Duration: 3 * time.Hour},
	}, {
		name:            "maximum pool lifetime set",
		maximumLifetime: &metav1.Duration{Duration: 1 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "claim lifetime set, and maximum pool lifetime set to higher",
		maximumLifetime: &metav1.Duration{Duration: 2 * time.Hour},
		claimLifetime:   &metav1.Duration{Duration: 1 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "claim lifetime set, and maximum pool lifetime set to lower",
		maximumLifetime: &metav1.Duration{Duration: 1 * time.Hour},
		claimLifetime:   &metav1.Duration{Duration: 2 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "default lifetime set, and maximum pool lifetime set to lower",
		defaultLifetime: &metav1.Duration{Duration: 2 * time.Hour},
		maximumLifetime: &metav1.Duration{Duration: 1 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}, {
		name:            "default lifetime set, and maximum pool lifetime set to higher",
		defaultLifetime: &metav1.Duration{Duration: 1 * time.Hour},
		maximumLifetime: &metav1.Duration{Duration: 2 * time.Hour},

		expected: &metav1.Duration{Duration: 1 * time.Hour},
	}}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var poolLifetime *hivev1.ClusterPoolClaimLifetime
			if test.defaultLifetime != nil {
				if poolLifetime == nil {
					poolLifetime = &hivev1.ClusterPoolClaimLifetime{}
				}
				poolLifetime.Default = test.defaultLifetime
			}
			if test.maximumLifetime != nil {
				if poolLifetime == nil {
					poolLifetime = &hivev1.ClusterPoolClaimLifetime{}
				}
				poolLifetime.Maximum = test.maximumLifetime
			}

			got := getClaimLifetime(poolLifetime, test.claimLifetime)
			assert.Equal(t, test.expected, got, "expected the lifetimes to match")
		})
	}
}

func testRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName,
			Name:      hiveClaimOwnerRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"hive.openshift.io"},
				Resources: []string{"*"},
				Verbs:     []string{"*"},
			},
			{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{kubeconfigSecretName, passwordSecretName},
				Verbs:         []string{"get"},
			},
		},
	}
}

func testRoleBinding() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterName,
			Name:      hiveClaimOwnerRoleBindingName,
		},
		Subjects: subjects,
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     hiveClaimOwnerRoleName,
		},
	}
}

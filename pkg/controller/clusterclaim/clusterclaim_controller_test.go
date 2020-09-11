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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	testclaim "github.com/openshift/hive/pkg/test/clusterclaim"
	testcd "github.com/openshift/hive/pkg/test/clusterdeployment"
	testgeneric "github.com/openshift/hive/pkg/test/generic"
)

const (
	claimNamespace       = "claim-namespace"
	claimName            = "test-claim"
	clusterName          = "test-cluster"
	kubeconfigSecretName = "kubeconfig-secret"
	passwordSecretName   = "password-secret"
)

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
	scheme := runtime.NewScheme()
	hivev1.AddToScheme(scheme)
	rbacv1.AddToScheme(scheme)

	claimBuilder := testclaim.FullBuilder(claimNamespace, claimName, scheme).Options(
		testclaim.WithSubjects(subjects),
	)
	cdBuilder := testcd.FullBuilder(clusterName, clusterName, scheme).Options(
		func(cd *hivev1.ClusterDeployment) {
			cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: kubeconfigSecretName},
				AdminPasswordSecretRef:   corev1.LocalObjectReference{Name: passwordSecretName},
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
			name:                 "new assignment",
			claim:                claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:                   cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			expectCompletedClaim: true,
			expectRBAC:           true,
		},
		{
			name:                 "existing assignment",
			claim:                claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:                   cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			expectCompletedClaim: true,
			expectRBAC:           true,
		},
		{
			name:               "assignment conflict",
			claim:              claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:                 cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			expectNoAssignment: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimPendingCondition,
				Status:  corev1.ConditionTrue,
				Reason:  "AssignmentConflict",
				Message: "Assigned cluster was claimed by a different ClusterClaim",
			}},
		},
		{
			name:  "deleted cluster",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
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
			claim: claimBuilder.Build(
				testclaim.WithCluster(clusterName),
				testclaim.WithCondition(
					hivev1.ClusterClaimCondition{
						Type:   hivev1.ClusterClaimPendingCondition,
						Status: corev1.ConditionTrue,
					},
				),
			),
			cd:                   cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimPendingCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterClaimed",
				Message: "Cluster claimed",
			}},
			expectRBAC: true,
		},
		{
			name: "deleted claim with no assignment",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(),
			expectNoAssignment:                     true,
			expectNoFinalizer:                      true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with unclaimed assignment",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectNoFinalizer: true,
			expectRBAC:        true,
		},
		{
			name: "deleted claim with claimed assignment",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim:                   true,
			expectNoFinalizer:                      true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with missing clusterdeployment",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			expectNoFinalizer:                      true,
			expectAssignedClusterDeploymentDeleted: true,
		},
		{
			name: "deleted claim with deleted clusterdeployment",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.GenericOptions(testgeneric.Deleted()).
				Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim: true,
			expectNoFinalizer:    true,
		},
		{
			name: "deleted claim with clusterdeployment assigned to other claim",
			claim: claimBuilder.GenericOptions(
				testgeneric.WithFinalizer(finalizer),
				testgeneric.Deleted(),
			).Build(testclaim.WithCluster(clusterName)),
			cd:                cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", "other-claim")),
			expectNoFinalizer: true,
		},
		{
			name:                 "no RBAC when no subjects",
			claim:                claimBuilder.Build(testclaim.WithCluster(clusterName), testclaim.WithSubjects(nil)),
			cd:                   cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			expectCompletedClaim: true,
		},
		{
			name:  "update existing role",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:    cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
			existing: []runtime.Object{
				func() runtime.Object {
					r := testRole()
					r.Rules = nil
					return r
				}(),
			},
			expectCompletedClaim: true,
			expectRBAC:           true,
		},
		{
			name:  "update existing rolebinding",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd:    cdBuilder.Build(testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool")),
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
		},
		{
			name:  "new assignment bring cluster out of hibernation",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithUnclaimedClusterPoolReference(claimNamespace, "test-pool"),
				testcd.WithPowerState(hivev1.HibernatingClusterPowerState),
			),
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectHibernating:    false,
		},
		{
			name:  "existing assignment does not change power state",
			claim: claimBuilder.Build(testclaim.WithCluster(clusterName)),
			cd: cdBuilder.Build(
				testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName),
				testcd.WithPowerState(hivev1.HibernatingClusterPowerState),
			),
			expectCompletedClaim: true,
			expectRBAC:           true,
			expectHibernating:    true,
		},
		{
			name: "claim with elapsed lifetime is deleted",
			claim: claimBuilder.Build(
				testclaim.WithCluster(clusterName),
				testclaim.WithLifetime(1*time.Hour),
				testclaim.WithCondition(hivev1.ClusterClaimCondition{
					Type:               hivev1.ClusterClaimPendingCondition,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
				}),
			),
			cd:            cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			expectDeleted: true,
		},
		{
			name: "claim with non-elapsed lifetime is not deleted",
			claim: claimBuilder.Build(
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
			cd: cdBuilder.Build(testcd.WithClusterPoolReference(claimNamespace, "test-pool", claimName)),
			existing: []runtime.Object{
				testRole(),
				testRoleBinding(),
			},
			expectCompletedClaim: true,
			expectedConditions: []hivev1.ClusterClaimCondition{{
				Type:    hivev1.ClusterClaimPendingCondition,
				Status:  corev1.ConditionFalse,
				Reason:  "ClusterClaimed",
				Message: "Cluster claimed",
			}},
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

			result, err := rcp.Reconcile(reconcileRequest)
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
			} else {
				assert.True(t, apierrors.IsNotFound(err), "expected claim to be deleted")
				return
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
					assignedClusterDeploymentExists = true
					if test.expectHibernating {
						assert.Equal(t, hivev1.HibernatingClusterPowerState, cd.Spec.PowerState, "expected ClusterDeployment to be hibernating")
					} else {
						assert.NotEqual(t, hivev1.HibernatingClusterPowerState, cd.Spec.PowerState, "expected ClusterDeployment to not be hibernating")
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

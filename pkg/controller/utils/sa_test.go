package utils

import (
	"context"
	"testing"

	testfake "github.com/openshift/hive/pkg/test/fake"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "test-namespace"
)

func TestSetupClusterInstallServiceAccount(t *testing.T) {
	cases := []struct {
		name     string
		existing []runtime.Object
	}{
		{
			name: "none",
		},
		{
			name: "all",
			existing: []runtime.Object{
				testServiceAccount(),
				testRole(),
				testRoleBinding(),
			},
		},
		{
			name: "only service account",
			existing: []runtime.Object{
				testServiceAccount(),
			},
		},
		{
			name: "only role",
			existing: []runtime.Object{
				testRole(),
			},
		},
		{
			name: "only role binding",
			existing: []runtime.Object{
				testRoleBinding(),
			},
		},
		{
			name: "no role rules",
			existing: []runtime.Object{
				testServiceAccount(),
				func() *rbacv1.Role {
					r := testRole()
					r.Rules = nil
					return r
				}(),
				testRoleBinding(),
			},
		},
		{
			name: "missing role rule",
			existing: []runtime.Object{
				testServiceAccount(),
				func() *rbacv1.Role {
					r := testRole()
					r.Rules = append(r.Rules[:1], r.Rules[2:]...)
					return r
				}(),
				testRoleBinding(),
			},
		},
		{
			name: "incorrect role rule",
			existing: []runtime.Object{
				testServiceAccount(),
				func() *rbacv1.Role {
					r := testRole()
					r.Rules[0].Verbs = []string{"bad-verb"}
					return r
				}(),
				testRoleBinding(),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := testfake.NewFakeClientBuilder().WithRuntimeObjects(tc.existing...).Build()
			err := SetupClusterInstallServiceAccount(fakeClient, testNamespace, log.StandardLogger())
			if !assert.NoError(t, err, "unexpected error setting up service account") {
				return
			}

			sa := &corev1.ServiceAccount{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: InstallServiceAccountName, Namespace: testNamespace}, sa)
			assert.NoError(t, err, "unexpected error fetching service account")

			role := &rbacv1.Role{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: installRoleName, Namespace: testNamespace}, role)
			if assert.NoError(t, err, "unexpected error fetching role") {
				assert.Equal(t, installRoleRules, role.Rules, "incorrect rules")
			}

			roleBinding := &rbacv1.RoleBinding{}
			err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: installRoleBindingName, Namespace: testNamespace}, roleBinding)
			if assert.NoError(t, err, "unexpected error fetching role binding") {
				if assert.Len(t, roleBinding.Subjects, 1, "unexpected number of subjects") {
					subject := roleBinding.Subjects[0]
					assert.Equal(t, "ServiceAccount", subject.Kind, "unexpected subject kind")
					assert.Equal(t, InstallServiceAccountName, subject.Name, "unexpected subject name")
					assert.Equal(t, testNamespace, subject.Namespace, "unexpected subject namespace")
				}
				assert.Equal(t, "Role", roleBinding.RoleRef.Kind, "unexpected role ref kind")
				assert.Equal(t, installRoleName, roleBinding.RoleRef.Name, "unexpected roel ref name")
			}
		})
	}
}

func testServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      InstallServiceAccountName,
			Namespace: testNamespace,
		},
	}
}

func testRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      installRoleName,
			Namespace: testNamespace,
		},
		Rules: installRoleRules,
	}
}

func testRoleBinding() *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      installRoleBindingName,
			Namespace: testNamespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      InstallServiceAccountName,
				Namespace: testNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Name: installRoleName,
			Kind: "Role",
		},
	}
}

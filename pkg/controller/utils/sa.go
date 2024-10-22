package utils

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// InstallServiceAccountName will be a service account that can run the installer and then
	// upload artifacts to the cluster's namespace.
	InstallServiceAccountName = "cluster-installer"
	installRoleName           = "cluster-installer"
	installRoleBindingName    = "cluster-installer"

	// UninstallServiceAccountName will be a service account that can run the installer deprovision and then
	// upload artifacts to the cluster's namespace.
	UninstallServiceAccountName = "cluster-uninstaller"
	uninstallRoleName           = "cluster-uninstaller"
	uninstallRoleBindingName    = "cluster-uninstaller"
)

var (
	installRoleRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
			Verbs:     []string{"create", "delete", "get", "list", "update"},
		},
		{
			APIGroups: []string{"hive.openshift.io"},
			Resources: []string{"dnszones"},
			Verbs:     []string{"get"},
		},
		{
			APIGroups: []string{"hive.openshift.io"},
			Resources: []string{"clusterdeployments", "clusterdeployments/finalizers", "clusterdeployments/status"},
			Verbs:     []string{"get", "update"},
		},
		{
			APIGroups: []string{"hive.openshift.io"},
			Resources: []string{"clusterprovisions", "clusterprovisions/finalizers", "clusterprovisions/status"},
			Verbs:     []string{"get", "list", "update", "watch"},
		},
		{
			APIGroups: []string{"hive.openshift.io"},
			Resources: []string{"machinepools"},
			Verbs:     []string{"get", "list", "update"},
		},
		{
			APIGroups: []string{"hive.openshift.io"},
			Resources: []string{"clusterdeploymentcustomizations"},
			Verbs:     []string{"get"},
		},
	}

	uninstallRoleRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
			Verbs:     []string{"create", "delete", "get", "list", "update"},
		},
	}
)

// SetupClusterInstallServiceAccount ensures a service account exists which can upload
// the required artifacts after running the installer in a pod. (metadata, admin kubeconfig)
func SetupClusterInstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) error {
	// create new serviceaccount if it doesn't already exist
	if err := setupServiceAccount(c, InstallServiceAccountName, namespace, logger); err != nil {
		return errors.Wrap(err, "failed to setup service account")
	}

	if err := setupRole(c, installRoleName, namespace, installRoleRules, logger); err != nil {
		return errors.Wrap(err, "failed to setup role")
	}

	// create rolebinding for the serviceaccount
	if err := setupRoleBinding(c, installRoleBindingName, namespace, installRoleName, InstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup rolebinding")
	}
	return nil
}

// SetupClusterUninstallServiceAccount ensures a service account exists which can read the required secrets, upload
// the required artifacts after running the installer deprovision in a pod.
func SetupClusterUninstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) error {
	// create new serviceaccount if it doesn't already exist
	if err := setupServiceAccount(c, UninstallServiceAccountName, namespace, logger); err != nil {
		return errors.Wrap(err, "failed to setup service account")
	}

	if err := setupRole(c, uninstallRoleName, namespace, uninstallRoleRules, logger); err != nil {
		return errors.Wrap(err, "failed to setup role")
	}

	// create rolebinding for the serviceaccount
	if err := setupRoleBinding(c, uninstallRoleBindingName, namespace, uninstallRoleName, UninstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup rolebinding")
	}
	return nil
}

func setupRoleBinding(c client.Client, name, namespace string, role, serviceaccount string, logger log.FieldLogger) error {
	switch err := c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, &rbacv1.RoleBinding{}); {
	case apierrors.IsNotFound(err):
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      serviceaccount,
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name: role,
				Kind: "Role",
			},
		}
		if err := c.Create(context.Background(), rb); err != nil {
			return errors.Wrap(err, "error creating rolebinding")
		}
		logger.WithField("name", name).Info("created rolebinding")
	case err != nil:
		return errors.Wrap(err, "error checking for existing rolebinding")
	default:
		logger.WithField("name", name).Debug("rolebinding already exists")
	}
	return nil
}

func setupRole(c client.Client, name, namespace string, rules []rbacv1.PolicyRule, logger log.FieldLogger) error {
	currentRole := &rbacv1.Role{}
	switch err := c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, currentRole); {
	case apierrors.IsNotFound(err):
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Rules: rules,
		}
		if err := c.Create(context.TODO(), role); err != nil {
			return errors.Wrap(err, "error creating role")
		}
		logger.WithField("name", name).Info("created role")
	case err != nil:
		return errors.Wrap(err, "error checking for existing role")
	case !reflect.DeepEqual(currentRole.Rules, rules):
		currentRole.Rules = rules
		if err := c.Update(context.TODO(), currentRole); err != nil {
			return errors.Wrap(err, "error updating role")
		}
		logger.WithField("name", name).Info("updated role")
	default:
		logger.WithField("name", name).Debug("role already exists")
	}
	return nil
}

func setupServiceAccount(c client.Client, name, namespace string, logger log.FieldLogger) error {
	switch err := c.Get(context.Background(), client.ObjectKey{Name: name, Namespace: namespace}, &corev1.ServiceAccount{}); {
	case apierrors.IsNotFound(err):
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
		}
		if err := c.Create(context.TODO(), sa); err != nil {
			return errors.Wrap(err, "error creating serviceaccount")
		}
		logger.WithField("name", name).Info("created service account")
	case err != nil:
		return errors.Wrap(err, "error checking for existing serviceaccount")
	default:
		logger.WithField("name", name).Debug("service account already exists")
	}
	return nil
}

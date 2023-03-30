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
	// Used for both Role and ClusterRole
	installRoleName = "cluster-installer"
	// Used for both RoleBinding and ClusterRoleBinding
	installRoleBindingName = "cluster-installer"

	// UninstallServiceAccountName will be a service account that can run the installer deprovision and then
	// upload artifacts to the cluster's namespace.
	UninstallServiceAccountName = "cluster-uninstaller"
	// Used for both Role and ClusterRole
	uninstallRoleName = "cluster-uninstaller"
	// Used for both RoleBinding and ClusterRoleBinding
	uninstallRoleBindingName = "cluster-uninstaller"
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
	}
	installClusterRoleRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"config.openshift.io"},
			Resources: []string{"proxies"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}

	uninstallRoleRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets", "configmaps"},
			Verbs:     []string{"create", "delete", "get", "list", "update"},
		},
	}
	uninstallClusterRoleRules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"config.openshift.io"},
			Resources: []string{"proxies"},
			Verbs:     []string{"get", "list", "watch"},
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

	if err := setupRoleBinding(c, installRoleBindingName, namespace, installRoleName, InstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup rolebinding")
	}

	if err := setupClusterRole(c, installRoleName, namespace, installClusterRoleRules, logger); err != nil {
		return errors.Wrap(err, "failed to setup clusterrole")
	}

	if err := setupClusterRoleBinding(c, installRoleBindingName, namespace, installRoleName, InstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup clusterrolebinding")
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

	if err := setupRoleBinding(c, uninstallRoleBindingName, namespace, uninstallRoleName, UninstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup rolebinding")
	}

	if err := setupClusterRole(c, uninstallRoleName, namespace, uninstallClusterRoleRules, logger); err != nil {
		return errors.Wrap(err, "failed to setup clusterrole")
	}

	if err := setupClusterRoleBinding(c, uninstallRoleBindingName, namespace, uninstallRoleName, UninstallServiceAccountName, logger); err != nil {
		return errors.Wrap(err, "failed to setup clusterrolebinding")
	}
	return nil
}

// DisableClusterInstallServiceAccount removes the Subject for the installer service account from the
// installer ClusterRoleBinding. It does not delete the service account (should it?).
func DisableClusterInstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) error {
	return removeCRBSubject(c, installRoleBindingName, namespace, InstallServiceAccountName, logger)
}

// DisableClusterUninstallServiceAccount removes the Subject for the uninstaller service account from the
// uninstaller ClusterRoleBinding. It does not delete the service account (should it?).
func DisableClusterUninstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) error {
	return removeCRBSubject(c, uninstallRoleBindingName, namespace, UninstallServiceAccountName, logger)
}

func removeCRBSubject(c client.Client, crbName, namespace, saName string, logger log.FieldLogger) error {
	logger = logger.
		WithField("name", crbName).
		WithField("namespace", namespace).
		WithField("serviceaccount", saName)
	crb := &rbacv1.ClusterRoleBinding{}
	if err := c.Get(context.Background(), client.ObjectKey{Name: crbName}, crb); err != nil {
		if apierrors.IsNotFound(err) {
			// If there's no such CRB, we have nothing to do
			logger.Debug("clusterrolebinding does not exist")
			return nil
		}
		// Any other error is an error
		return errors.Wrapf(err, "error retrieving %s clusterrolebinding", crbName)
	}
	return ensureCRBSubject(c, crb, saName, namespace, false, logger)
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

func setupClusterRoleBinding(c client.Client, name, namespace string, role, serviceaccount string, logger log.FieldLogger) error {
	logger = logger.WithField("name", name).WithField("namespace", namespace).WithField("serviceaccount", serviceaccount)
	rb := &rbacv1.ClusterRoleBinding{}
	switch err := c.Get(context.Background(), client.ObjectKey{Name: name}, rb); {
	case apierrors.IsNotFound(err):
		rb = &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
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
				Kind: "ClusterRole",
			},
		}
		if err := c.Create(context.Background(), rb); err != nil {
			return errors.Wrap(err, "error creating clusterrolebinding")
		}
		logger.Info("created clusterrolebinding")
	case err != nil:
		return errors.Wrap(err, "error checking for existing clusterrolebinding")
	default:
		logger.Debug("clusterrolebinding already exists")
		if err := ensureCRBSubject(c, rb, serviceaccount, namespace, true, logger); err != nil {
			return errors.Wrapf(err, "error adding serviceaccount %s to clusterrolebinding", serviceaccount)
		}
	}
	return nil
}

// ensureCRBSubject idempotently ensures that `crb.Subjects` either contains or does not contain
// (according to `wantPresent`) a "ServiceAccount" Subject with the given `namespace` and `sa`
// Name. Returns the error from Update().
func ensureCRBSubject(c client.Client, crb *rbacv1.ClusterRoleBinding, sa, namespace string, wantPresent bool, logger log.FieldLogger) error {
	exp := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      sa,
		Namespace: namespace,
	}
	newSubjects := []rbacv1.Subject{}
	for _, subject := range crb.Subjects {
		if subject == exp {
			if wantPresent {
				logger.Debug("clusterrolebinding already contains subject for serviceaccount")
				return nil
			}
			// We want this one absent, so skip adding it to newSubjects
			logger.Info("removing subject for serviceaccount from clusterrolebinding")
			continue
		}
		newSubjects = append(newSubjects, subject)
	}

	if wantPresent {
		// If we wanted it present and it already was, we already returned; so if we get here, we need to add it.
		logger.Info("adding subject for serviceaccount to clusterrolebinding")
		newSubjects = append(newSubjects, exp)
	}

	if len(newSubjects) == len(crb.Subjects) {
		logger.Debug("clusterrolebinding already contains no subject for serviceaccount")
		return nil
	}

	crb.Subjects = newSubjects
	return c.Update(context.Background(), crb)
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

// TODO: Collapse with setupRole when generics can do common fields: https://github.com/golang/go/issues/48522
func setupClusterRole(c client.Client, name, namespace string, rules []rbacv1.PolicyRule, logger log.FieldLogger) error {
	currentRole := &rbacv1.ClusterRole{}
	switch err := c.Get(context.Background(), client.ObjectKey{Name: name}, currentRole); {
	case apierrors.IsNotFound(err):
		role := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Rules: rules,
		}
		if err := c.Create(context.TODO(), role); err != nil {
			return errors.Wrap(err, "error creating clusterrole")
		}
		logger.WithField("name", name).Info("created clusterrole")
	case err != nil:
		return errors.Wrap(err, "error checking for existing clusterrole")
	case !reflect.DeepEqual(currentRole.Rules, rules):
		currentRole.Rules = rules
		if err := c.Update(context.TODO(), currentRole); err != nil {
			return errors.Wrap(err, "error updating clusterrole")
		}
		logger.WithField("name", name).Info("updated clusterrole")
	default:
		logger.WithField("name", name).Debug("clusterrole already exists")
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

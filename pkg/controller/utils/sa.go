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
	// ServiceAccountName will be a service account that can run the installer and then
	// upload artifacts to the cluster's namespace.
	ServiceAccountName = "cluster-installer"
	roleName           = "cluster-installer"
	roleBindingName    = "cluster-installer"
)

var (
	roleRules = []rbacv1.PolicyRule{
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
	}
)

// SetupClusterInstallServiceAccount ensures a service account exists which can upload
// the required artifacts after running the installer in a pod. (metadata, admin kubeconfig)
func SetupClusterInstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) error {
	// create new serviceaccount if it doesn't already exist
	switch err := c.Get(context.Background(), client.ObjectKey{Name: ServiceAccountName, Namespace: namespace}, &corev1.ServiceAccount{}); {
	case apierrors.IsNotFound(err):
		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ServiceAccountName,
				Namespace: namespace,
			},
		}
		if err := c.Create(context.TODO(), sa); err != nil {
			return errors.Wrap(err, "error creating serviceaccount")
		}
		logger.WithField("name", ServiceAccountName).Info("created service account")
	case err != nil:
		return errors.Wrap(err, "error checking for existing serviceaccount")
	default:
		logger.WithField("name", ServiceAccountName).Debug("service account already exists")
	}

	currentRole := &rbacv1.Role{}
	switch err := c.Get(context.Background(), client.ObjectKey{Name: roleName, Namespace: namespace}, currentRole); {
	case apierrors.IsNotFound(err):
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: namespace,
			},
			Rules: roleRules,
		}
		if err := c.Create(context.TODO(), role); err != nil {
			return errors.Wrap(err, "error creating role")
		}
		logger.WithField("name", roleName).Info("created role")
	case err != nil:
		return errors.Wrap(err, "error checking for existing role")
	case !reflect.DeepEqual(currentRole.Rules, roleRules):
		currentRole.Rules = roleRules
		if err := c.Update(context.TODO(), currentRole); err != nil {
			return errors.Wrap(err, "error updating role")
		}
		logger.WithField("name", roleName).Info("updated role")
	default:
		logger.WithField("name", roleName).Debug("role already exists")
	}

	// create rolebinding for the serviceaccount
	switch err := c.Get(context.Background(), client.ObjectKey{Name: roleBindingName, Namespace: namespace}, &rbacv1.RoleBinding{}); {
	case apierrors.IsNotFound(err):
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleBindingName,
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      ServiceAccountName,
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name: roleName,
				Kind: "Role",
			},
		}
		if err := c.Create(context.Background(), rb); err != nil {
			return errors.Wrap(err, "error creating rolebinding")
		}
		logger.WithField("name", roleBindingName).Info("created rolebinding")
	case err != nil:
		return errors.Wrap(err, "error checking for existing rolebinding")
	default:
		logger.WithField("name", roleBindingName).Debug("rolebinding already exists")
	}

	return nil
}

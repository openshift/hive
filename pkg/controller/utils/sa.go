/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// serviceAccountName will be a service account that can run the installer and then
	// upload artifacts to the cluster's namespace.
	serviceAccountName = "cluster-installer"
	roleName           = "cluster-installer"
	roleBindingName    = "cluster-installer"
)

// SetupClusterInstallServiceAccount ensures a service account exists which can upload
// the required artifacts after running the installer in a pod. (metadata, admin kubeconfig)
func SetupClusterInstallServiceAccount(c client.Client, namespace string, logger log.FieldLogger) (*corev1.ServiceAccount, error) {
	// create new serviceaccount if it doesn't already exist
	currentSA := &corev1.ServiceAccount{}
	err := c.Get(context.Background(), client.ObjectKey{Name: serviceAccountName, Namespace: namespace}, currentSA)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing serviceaccount")
	}

	if errors.IsNotFound(err) {
		currentSA = &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		}
		err = c.Create(context.Background(), currentSA)

		if err != nil {
			return nil, fmt.Errorf("error creating serviceaccount: %v", err)
		}
		logger.WithField("name", serviceAccountName).Info("created service account")
	} else {
		logger.WithField("name", serviceAccountName).Debug("service account already exists")
	}

	expectedRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"secrets", "configmaps"},
				Verbs:     []string{"create", "delete", "get", "list", "update"},
			},
			{
				APIGroups: []string{"hive.openshift.io"},
				Resources: []string{"clusterdeployments", "clusterdeployments/finalizers", "clusterdeployments/status"},
				Verbs:     []string{"create", "delete", "get", "list", "update"},
			},
		},
	}
	currentRole := &rbacv1.Role{}
	err = c.Get(context.Background(), client.ObjectKey{Name: roleName, Namespace: namespace}, currentRole)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing role: %v", err)
	}
	if errors.IsNotFound(err) {
		err = c.Create(context.Background(), expectedRole)
		if err != nil {
			return nil, fmt.Errorf("error creating role: %v", err)
		}
		logger.WithField("name", roleName).Info("created role")
	} else {
		logger.WithField("name", roleName).Debug("role already exists")
	}

	// create rolebinding for the serviceaccount
	currentRB := &rbacv1.RoleBinding{}
	err = c.Get(context.Background(), client.ObjectKey{Name: roleBindingName, Namespace: namespace}, currentRB)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing rolebinding: %v", err)
	}

	if errors.IsNotFound(err) {
		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleBindingName,
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      currentSA.Name,
					Namespace: namespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Name: roleName,
				Kind: "Role",
			},
		}

		err = c.Create(context.Background(), rb)
		if err != nil {
			return nil, fmt.Errorf("error creating rolebinding: %v", err)
		}
		logger.WithField("name", roleBindingName).Info("created rolebinding")
	} else {
		logger.WithField("name", roleBindingName).Debug("rolebinding already exists")
	}

	return currentSA, nil
}

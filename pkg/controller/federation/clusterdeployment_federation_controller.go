/*
Copyright 2018 The Kubernetes Authors.

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

package federation

import (
	"context"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	federationjob "github.com/openshift/hive/pkg/federation"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

const (
	// serviceAccountName will be a service account that can federate a target cluster
	serviceAccountName = "cluster-federator"
	roleName           = "cluster-admin"
	roleBindingPrefix  = "cluster-federator"

	adminKubeconfigKey = "kubeconfig"

	federatedClustersCRDName = "federatedclusters.core.federation.k8s.io"
)

// Add creates a new ClusterDeployment Federation Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeploymentFederation{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeployment-federation-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for jobs created for a ClusterDeployment:
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeploymentFederation{}

// ReconcileClusterDeploymentFederation reconciles a ClusterDeployment object
type ReconcileClusterDeploymentFederation struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and federates it if it's installed
// and federation is present in the cluster.
//
// Automatically generate RBAC rules to allow the Controller to read and write ClusterDeployments
//
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
func (r *ReconcileClusterDeploymentFederation) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	federationInstalled, err := r.isFederationInstalled()
	if err != nil || !federationInstalled {
		log.Debug("Cluster deployment federation: federation not installed, nothing to do.")
		return reconcile.Result{}, err
	}

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err = r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	cdLog := log.WithFields(log.Fields{
		"controller":        "cluster-deployment-federation",
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	// Filter on deleted clusterdeployments or ones that are not
	// installed yet.
	if cd.DeletionTimestamp != nil ||
		!cd.Status.Installed ||
		cd.Status.AdminKubeconfigSecret.Name == "" {
		cdLog.Debug("cluster deployment not ready for federation")
		return reconcile.Result{}, nil
	}

	// If already federated, skip
	if cd.Status.Federated {
		cdLog.Debug("cluster already federated, nothing to do")
		return reconcile.Result{}, nil
	}

	cdLog.Info("reconciling cluster deployment for federation")

	_, err = r.setupClusterFederationServiceAccount(cd.Namespace, cdLog)
	if err != nil {
		cdLog.WithError(err).Error("error setting up service account and role binding")
		return reconcile.Result{}, err
	}

	// Obtain cluster's kubeconfig secret
	kubeconfig, err := r.loadSecretData(cd.Status.AdminKubeconfigSecret.Name, cd.Namespace, adminKubeconfigKey)
	if err != nil {
		cdLog.WithError(err).Error("error retrieving kubeconfig for cluster")
		return reconcile.Result{}, err
	}

	job, secret, err := federationjob.GenerateFederationJob(
		cd,
		[]byte(kubeconfig),
		serviceAccountName)
	if err != nil {
		cdLog.WithError(err).Error("error generating federation job")
		return reconcile.Result{}, err
	}

	if err = controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on job")
		return reconcile.Result{}, err
	}
	if err = controllerutil.SetControllerReference(cd, secret, r.scheme); err != nil {
		cdLog.WithError(err).Error("error setting controller reference on secret")
		return reconcile.Result{}, err
	}

	cdLog = cdLog.WithField("job", job.Name)

	cdLog.Debug("checking if kubeconfig secret exists")
	existingSecret := &corev1.Secret{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, existingSecret)
	if err != nil && errors.IsNotFound(err) {
		cdLog.WithField("secret", secret.Name).Infof("creating secret")
		err = r.Create(context.TODO(), secret)
		if err != nil {
			cdLog.WithError(err).Error("error creating secret")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		cdLog.WithError(err).Error("error getting secret")
		return reconcile.Result{}, err
	}

	// Check if the Job already exists for this ClusterDeployment:
	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		// If the ClusterDeployment is already federated, we do not need to create a new job:
		if cd.Status.Federated {
			cdLog.Debug("cluster is already federated, no job needed")
		} else {
			cdLog.Infof("creating federation job")
			err = r.Create(context.TODO(), job)
			if err != nil {
				cdLog.WithError(err).Errorf("error creating job")
				return reconcile.Result{}, err
			}
		}
	} else if err != nil {
		cdLog.WithError(err).Error("error getting job")
		return reconcile.Result{}, err
	} else {
		cdLog.Infof("federation job exists, successful: %v", cd.Status.Federated)
	}

	err = r.updateClusterDeploymentStatus(cd, existingJob, cdLog)
	if err != nil {
		cdLog.WithError(err).Errorf("error updating cluster deployment status")
		return reconcile.Result{}, err
	}

	cdLog.Debugf("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeploymentFederation) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

func (r *ReconcileClusterDeploymentFederation) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, job *batchv1.Job, cdLog log.FieldLogger) error {
	cdLog.Debug("updating cluster deployment status")
	origCD := cd
	cd = cd.DeepCopy()
	if job != nil && job.Name != "" && job.Namespace != "" {
		// Job exists, check it's status:
		cd.Status.Federated = isSuccessful(job)
	}

	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment")
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment: %v", err)
			return err
		}
	} else {
		cdLog.Infof("cluster deployment status unchanged")
	}
	return nil
}

func (r *ReconcileClusterDeploymentFederation) isFederationInstalled() (bool, error) {
	crd := &apiextv1.CustomResourceDefinition{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: federatedClustersCRDName}, crd)
	if err != nil && !errors.IsNotFound(err) {
		return false, err
	}
	return err == nil, nil
}

// setupClusterFederationServiceAccount ensures a service account exists which can federate a target cluster
func (r *ReconcileClusterDeploymentFederation) setupClusterFederationServiceAccount(namespace string, cdLog log.FieldLogger) (*corev1.ServiceAccount, error) {
	// create new serviceaccount if it doesn't already exist
	currentSA := &corev1.ServiceAccount{}
	err := r.Client.Get(context.Background(), client.ObjectKey{Name: serviceAccountName, Namespace: namespace}, currentSA)
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
		err = r.Client.Create(context.Background(), currentSA)

		if err != nil {
			return nil, fmt.Errorf("error creating serviceaccount: %v", err)
		}
		cdLog.WithField("name", serviceAccountName).Info("created service account")
	} else {
		cdLog.WithField("name", serviceAccountName).Debug("service account already exists")
	}

	// create rolebinding for the serviceaccount
	currentRB := &rbacv1.ClusterRoleBinding{}
	roleBindingName := fmt.Sprintf("%s-%s", roleBindingPrefix, namespace)
	err = r.Client.Get(context.Background(), client.ObjectKey{Name: roleBindingName}, currentRB)

	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("error checking for existing clusterrolebinding: %v", err)
	}

	if errors.IsNotFound(err) {
		rb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleBindingName,
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
				Kind: "ClusterRole",
			},
		}

		err = r.Client.Create(context.Background(), rb)
		if err != nil {
			return nil, fmt.Errorf("error creating rolebinding: %v", err)
		}
		cdLog.WithField("name", roleBindingName).Info("created rolebinding")
	} else {
		cdLog.WithField("name", roleBindingName).Debug("rolebinding already exists")
	}

	return currentSA, nil
}

// getJobConditionStatus gets the status of the condition in the job. If the
// condition is not found in the job, then returns False.
func getJobConditionStatus(job *batchv1.Job, conditionType batchv1.JobConditionType) corev1.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionFalse
}

func isSuccessful(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobComplete) == corev1.ConditionTrue
}

func isFailed(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobFailed) == corev1.ConditionTrue
}

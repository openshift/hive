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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"

	federationutil "github.com/kubernetes-sigs/federation-v2/pkg/controller/util"
	"github.com/kubernetes-sigs/federation-v2/pkg/kubefed2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
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
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=clusterregistry.k8s.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.federation.k8s.io,resources=federatedclusters,verbs=get;list;watch;create;update;patch;delete
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

	// Obtain cluster's kubeconfig secret
	kubeconfig, err := r.loadSecretData(cd.Status.AdminKubeconfigSecret.Name, cd.Namespace, adminKubeconfigKey)
	if err != nil {
		cdLog.WithError(err).Error("error retrieving kubeconfig for cluster")
		return reconcile.Result{}, err
	}

	hostConfig, err := config.GetConfig()
	if err != nil {
		cdLog.WithError(err).Error("cannot obtain host client configuration")
		return reconcile.Result{}, err
	}

	targetConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		cdLog.WithError(err).Error("cannot create target cluster client config")
		return reconcile.Result{}, err
	}

	err = kubefed2.JoinCluster(hostConfig,
		targetConfig,
		federationutil.DefaultFederationSystemNamespace,
		federationutil.MulticlusterPublicNamespace,
		"hive",  /* hostContext */
		cd.Name, /* clusterName */
		"",      /* secretName */
		true,    /* addToRegistry */
		false,   /* limitedScope */
		false,   /* dryRun */
		true)    /* idempotent */

	if err != nil {
		cdLog.WithError(err).Error("Federating cluster failed")
		// TODO: Until the join command is idempotent, returning error here will only
		// result in a quick backoff loop.
		return reconcile.Result{}, nil
	}

	err = r.updateClusterDeploymentStatus(cd, cdLog)
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

func (r *ReconcileClusterDeploymentFederation) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	cdLog.Debug("updating cluster deployment status")
	origCD := cd
	cd = cd.DeepCopy()

	cd.Status.Federated = true

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

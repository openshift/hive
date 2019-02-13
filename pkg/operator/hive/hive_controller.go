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

package hive

import (
	"context"

	log "github.com/sirupsen/logrus"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/operator/assets"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	oappsv1 "github.com/openshift/api/apps/v1"

	//apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextclientv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	legacyDeploymentConfig = "hive-controller-manager"
)

// Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHive{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hive-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileHive).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHive).apiextClient, err = apiextclientv1beta1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Watch for changes to Hive
	err = c.Watch(&source.Kind{Type: &hivev1alpha1.Hive{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by Hive - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1alpha1.Hive{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHive{}

// ReconcileHive reconciles a Hive object
type ReconcileHive struct {
	client.Client
	scheme       *runtime.Scheme
	kubeClient   kubernetes.Interface
	apiextClient *apiextclientv1beta1.ApiextensionsV1beta1Client
}

// Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read
// and what is in the Hive.Spec
func (r *ReconcileHive) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	hLog := log.WithField("controller", "hive")
	hLog.Info("Reconciling Hive components")

	// Fetch the Hive instance
	instance := &hivev1alpha1.Hive{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	recorder := events.NewRecorder(r.kubeClient.CoreV1().Events(request.Namespace), "hive-operator", &corev1.ObjectReference{
		Name:      request.Name,
		Namespace: request.Namespace,
	})

	// Ensure legacy DeploymentConfig is deleted, we switched to a Deployment:
	dc := &oappsv1.DeploymentConfig{}
	err = r.Get(context.Background(), types.NamespacedName{Name: legacyDeploymentConfig, Namespace: "openshift-hive"}, dc)
	if err != nil && !errors.IsNotFound(err) {
		hLog.WithError(err).Error("error looking up legacy DeploymentConfig")
		return reconcile.Result{}, err
	} else if err != nil {
		hLog.WithField("DeploymentConfig", legacyDeploymentConfig).Debug("legacy DeploymentConfig does not exist")
	} else {
		err = r.Delete(context.Background(), dc)
		if err != nil {
			hLog.WithError(err).WithField("DeploymentConfig", legacyDeploymentConfig).Error(
				"error deleting legacy DeploymentConfig")
			return reconcile.Result{}, err
		}
		hLog.WithField("DeploymentConfig", legacyDeploymentConfig).Info("deleted legacy DeploymentConfig")
	}

	// Parse yaml for all Hive objects:
	asset, err := assets.Asset("config/crds/hive_v1alpha1_clusterdeployment.yaml")
	if err != nil {
		hLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	hLog.Debug("reading ClusterDeployment CRD")
	clusterDeploymentCRD := resourceread.ReadCustomResourceDefinitionV1Beta1OrDie(asset)

	asset, err = assets.Asset("config/crds/hive_v1alpha1_dnszone.yaml")
	if err != nil {
		hLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	hLog.Debug("reading DNSZone CRD")
	dnsZoneCRD := resourceread.ReadCustomResourceDefinitionV1Beta1OrDie(asset)

	asset, err = assets.Asset("config/manager/service.yaml")
	if err != nil {
		hLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	hLog.Debug("reading service")
	hiveSvc := resourceread.ReadServiceV1OrDie(asset)

	asset, err = assets.Asset("config/manager/deployment.yaml")
	if err != nil {
		hLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	hLog.Debug("reading deployment")
	hiveDeployment := resourceread.ReadDeploymentV1OrDie(asset)

	// Set owner refs on all objects in the deployment so deleting the operator CRD
	// will clean everything up:
	// NOTE: we do not cleanup the CRDs themselves so as not to destroy data.
	if err := controllerutil.SetControllerReference(instance, hiveSvc, r.scheme); err != nil {
		hLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, hiveDeployment, r.scheme); err != nil {
		hLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	_, changed, err := resourceapply.ApplyCustomResourceDefinition(r.apiextClient,
		recorder, clusterDeploymentCRD)
	if err != nil {
		hLog.WithError(err).Error("error applying ClusterDeployment CRD")
		return reconcile.Result{}, err
	} else {
		hLog.WithField("changed", changed).Info("ClusterDeployment CRD updated")
	}

	_, changed, err = resourceapply.ApplyCustomResourceDefinition(r.apiextClient,
		recorder, dnsZoneCRD)
	if err != nil {
		hLog.WithError(err).Error("error applying DNSZone CRD")
		return reconcile.Result{}, err
	} else {
		hLog.WithField("changed", changed).Info("DNSZone CRD updated")
	}

	_, changed, err = resourceapply.ApplyService(r.kubeClient.CoreV1(),
		recorder, hiveSvc)
	if err != nil {
		hLog.WithError(err).Error("error applying service")
		return reconcile.Result{}, err
	} else {
		hLog.WithField("changed", changed).Info("service updated")
	}

	expectedDeploymentGen := int64(0)
	currentDeployment := &appsv1.Deployment{}
	err = r.Get(context.Background(), types.NamespacedName{Name: hiveDeployment.Name, Namespace: hiveDeployment.Namespace}, currentDeployment)
	if err != nil && !errors.IsNotFound(err) {
		hLog.WithError(err).Error("error looking up current deployment")
		return reconcile.Result{}, err
	} else if err == nil {
		expectedDeploymentGen = currentDeployment.ObjectMeta.Generation
	}

	_, changed, err = resourceapply.ApplyDeployment(r.kubeClient.AppsV1(),
		recorder, hiveDeployment, expectedDeploymentGen, false)
	if err != nil {
		hLog.WithError(err).Error("error applying deployment")
		return reconcile.Result{}, err
	} else {
		hLog.WithField("changed", changed).Info("deployment updated")
	}

	hLog.Info("Hive components reconciled")

	return reconcile.Result{}, nil
}

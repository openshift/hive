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

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	"github.com/openshift/library-go/pkg/operator/events"

	oappsv1 "github.com/openshift/api/apps/v1"

	apiextclientv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	apiregclientv1 "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	legacyDeploymentConfig = "hive-controller-manager"
	legacyService          = "hive-controller-manager-service"
	// hiveNamespace is the assumed and only supported namespace where Hive will be deployed.
	hiveNamespace = "openshift-hive"
	// hiveConfigName is the one and only name for a HiveConfig supported in the cluster. Any others will be ignored.
	hiveConfigName = "hive"
)

// Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveConfig{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hive-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).apiregClient, err = apiregclientv1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).apiextClient, err = apiextclientv1beta1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Watch for changes to HiveConfig:
	err = c.Watch(&source.Kind{Type: &hivev1.HiveConfig{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Monitor changes to DaemonSets:
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Deployments:
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Services:
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// TODO: Monitor CRDs but do not try to use an owner ref. (as they are global,
	// and our config is namespaced)

	// TODO: it would be nice to monitor the global resources ValidatingWebhookConfiguration
	// and APIService, CRDs, but these cannot have OwnerReferences (which are not namespaced) as they
	// are global. Need to use a different predicate to the Watch function.

	return nil
}

var _ reconcile.Reconciler = &ReconcileHiveConfig{}

// ReconcileHiveConfig reconciles a Hive object
type ReconcileHiveConfig struct {
	client.Client
	scheme       *runtime.Scheme
	kubeClient   kubernetes.Interface
	apiextClient *apiextclientv1beta1.ApiextensionsV1beta1Client
	apiregClient *apiregclientv1.ApiregistrationV1Client
}

// Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read
// and what is in the Hive.Spec
func (r *ReconcileHiveConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	hLog := log.WithField("controller", "hive")
	hLog.Info("Reconciling Hive components")

	// Fetch the Hive instance
	instance := &hivev1.HiveConfig{}
	// NOTE: ignoring the Namespace that seems to get set on request when syncing on namespaced objects,
	// when our HiveConfig is ClusterScoped.
	err := r.Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			hLog.Debug("HiveConfig not found, deleted?")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		hLog.WithError(err).Error("error reading HiveConfig")
		return reconcile.Result{}, err
	}

	// We only support one HiveConfig per cluster, and it must be called "hive". This prevents installing
	// Hive more than once in the cluster.
	if instance.Name != hiveConfigName {
		hLog.WithField("hiveConfig", instance.Name).Warn("invalid HiveConfig name, only one HiveConfig supported per cluster and must be named 'hive'")
		return reconcile.Result{}, nil
	}

	recorder := events.NewRecorder(r.kubeClient.CoreV1().Events(request.Namespace), "hive-operator", &corev1.ObjectReference{
		Name:      request.Name,
		Namespace: hiveNamespace,
	})

	err = r.deleteLegacyComponents(hLog)
	if err != nil {
		hLog.WithError(err).Error("error deleting legacy components")
		return reconcile.Result{}, err
	}

	err = r.deployHive(hLog, instance, recorder)
	if err != nil {
		hLog.WithError(err).Error("error deploying Hive")
		return reconcile.Result{}, err
	}

	err = r.deployHiveAdmission(hLog, instance, recorder)
	if err != nil {
		hLog.WithError(err).Error("error deploying HiveAdmission")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// deleteLegacyComponents deletes Hive components that once existed but have since
// been removed or renamed.
func (r *ReconcileHiveConfig) deleteLegacyComponents(hLog log.FieldLogger) error {
	// Ensure legacy DeploymentConfig is deleted, we switched to a Deployment:
	// TODO: this can be removed once rolled out to opshive, our only persistent environment.
	dc := &oappsv1.DeploymentConfig{}
	err := r.Get(context.Background(), types.NamespacedName{Name: legacyDeploymentConfig, Namespace: hiveNamespace}, dc)
	if err != nil && !errors.IsNotFound(err) {
		hLog.WithError(err).Error("error looking up legacy DeploymentConfig")
		return err
	} else if err != nil {
		hLog.WithField("DeploymentConfig", legacyDeploymentConfig).Debug("legacy DeploymentConfig does not exist")
	} else {
		err = r.Delete(context.Background(), dc)
		if err != nil {
			hLog.WithError(err).WithField("DeploymentConfig", legacyDeploymentConfig).Error(
				"error deleting legacy DeploymentConfig")
			return err
		}
		hLog.WithField("DeploymentConfig", legacyDeploymentConfig).Info("deleted legacy DeploymentConfig")
	}

	// Ensure legacy Service is deleted, renamed.
	// TODO: this can be removed once rolled out to opshive, our only persistent environment.
	oldSvc := &corev1.Service{}
	err = r.Get(context.Background(), types.NamespacedName{Name: legacyService, Namespace: hiveNamespace}, oldSvc)
	if err != nil && !errors.IsNotFound(err) {
		hLog.WithError(err).Error("error looking up legacy Service")
		return err
	} else if err != nil {
		hLog.WithField("Service", legacyService).Debug("legacy Service does not exist")
	} else {
		err = r.Delete(context.Background(), oldSvc)
		if err != nil {
			hLog.WithError(err).WithField("Service", legacyService).Error(
				"error deleting legacy Service")
			return err
		}
		hLog.WithField("Service", legacyService).Info("deleted legacy Service")
	}
	return nil
}

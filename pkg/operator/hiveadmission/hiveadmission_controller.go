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

package hiveadmission

import (
	"context"

	log "github.com/sirupsen/logrus"

	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/operator/assets"

	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	apiregclientv1 "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// hiveAdmissionConfigName is the one and only name for a HiveAdmissionConfig supported in the cluster. Any others will be ignored.
	hiveAdmissionConfigName = "hiveadmission"
)

// Add creates a new HiveAdmissionConfig Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveAdmissionConfig{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hiveadmission-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileHiveAdmissionConfig).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveAdmissionConfig).apiregClient, err = apiregclientv1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Watch for changes to HiveAdmissionConfig
	err = c.Watch(&source.Kind{Type: &hivev1alpha1.HiveAdmissionConfig{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by HiveAdmissionConfig - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1alpha1.HiveAdmissionConfig{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHiveAdmissionConfig{}

// ReconcileHiveAdmissionConfig reconciles a HiveAdmissionConfig object
type ReconcileHiveAdmissionConfig struct {
	client.Client
	scheme       *runtime.Scheme
	kubeClient   kubernetes.Interface
	apiregClient *apiregclientv1.ApiregistrationV1Client
}

// Reconcile reads that state of the cluster for a HiveAdmissionConfig object and makes changes based on the state read
// and what is in the HiveAdmissionConfig.Spec
func (r *ReconcileHiveAdmissionConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	haLog := log.WithField("controller", "hiveadmission")
	haLog.Info("Reconciling HiveAdmissionConfig components")

	// Fetch the HiveAdmissionConfig instance
	instance := &hivev1alpha1.HiveAdmissionConfig{}
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

	// We only support one HiveAdmissionConfig per cluster, and it must be called "hiveadmission". This prevents installing
	// Hive more than once in the cluster.
	if instance.Name != hiveAdmissionConfigName {
		haLog.WithField("hiveAdmissionConfig", instance.Name).Warn("invalid HiveAdmissionConfig name, only one HiveAdmissionConfig supported per cluster and must be named 'hiveadmission'")
		return reconcile.Result{}, nil
	}

	recorder := events.NewRecorder(r.kubeClient.CoreV1().Events(request.Namespace), "hive-operator", &corev1.ObjectReference{
		Name:      request.Name,
		Namespace: request.Namespace,
	})

	asset, err := assets.Asset("config/hiveadmission/service.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading service")
	hiveAdmService := resourceread.ReadServiceV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/service-account.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading service account")
	hiveAdmServiceAcct := resourceread.ReadServiceAccountV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/daemonset.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading daemonset")
	hiveAdmDaemonSet := resourceread.ReadDaemonSetV1OrDie(asset)

	asset, err = assets.Asset("config/hiveadmission/apiservice.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading apiservice")
	hiveAdmAPIService := ReadAPIServiceV1Beta1OrDie(asset, r.scheme)

	asset, err = assets.Asset("config/hiveadmission/dnszones-webhook.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading DNSZones webhook")
	dnsZonesWebhook := ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	asset, err = assets.Asset("config/hiveadmission/clusterdeployment-webhook.yaml")
	if err != nil {
		haLog.WithError(err).Error("error loading asset")
		return reconcile.Result{}, err
	}
	haLog.Debug("reading ClusterDeployment webhook")
	cdWebhook := ReadValidatingWebhookConfigurationV1Beta1OrDie(asset, r.scheme)

	// Set owner refs on all objects in the deployment so deleting the operator CRD
	// will clean everything up:
	if err := controllerutil.SetControllerReference(instance, hiveAdmService, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, dnsZonesWebhook, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, cdWebhook, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, hiveAdmServiceAcct, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, hiveAdmDaemonSet, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(instance, hiveAdmAPIService, r.scheme); err != nil {
		haLog.WithError(err).Info("error setting owner ref")
		return reconcile.Result{}, err
	}

	_, changed, err := resourceapply.ApplyService(r.kubeClient.CoreV1(), recorder, hiveAdmService)
	if err != nil {
		haLog.WithError(err).Error("error applying service")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("service updated")
	}

	_, changed, err = resourceapply.ApplyServiceAccount(r.kubeClient.CoreV1(), recorder, hiveAdmServiceAcct)
	if err != nil {
		haLog.WithError(err).Error("error applying service account")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("service account updated")
	}

	expectedDSGen := int64(0)
	currentDS := &appsv1.DaemonSet{}
	err = r.Get(context.Background(), types.NamespacedName{Name: hiveAdmDaemonSet.Name, Namespace: hiveAdmDaemonSet.Namespace}, currentDS)
	if err != nil && !errors.IsNotFound(err) {
		haLog.WithError(err).Error("error looking up current daemonset")
		return reconcile.Result{}, err
	} else if err == nil {
		expectedDSGen = currentDS.ObjectMeta.Generation
	}

	if instance.Spec.Image != "" {
		haLog.WithFields(log.Fields{
			"orig": hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image,
			"new":  instance.Spec.Image,
		}).Info("overriding deployment image")
		hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image = instance.Spec.Image
	}
	haLog.WithFields(log.Fields{
		"orig": hiveAdmDaemonSet.Spec.Template.Spec.Containers[0].Image,
	}).Info("did it work?")

	_, changed, err = resourceapply.ApplyDaemonSet(r.kubeClient.AppsV1(), recorder, hiveAdmDaemonSet, expectedDSGen, false)
	if err != nil {
		haLog.WithError(err).Error("error applying daemonset")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("daemonset updated")
	}

	_, changed, err = resourceapply.ApplyAPIService(r.apiregClient, hiveAdmAPIService)
	if err != nil {
		haLog.WithError(err).Error("error applying apiservice")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("apiservice updated")
	}

	_, changed, err = ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), dnsZonesWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying DNSZones webhook")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("DNSZones webhook updated")
	}

	_, changed, err = ApplyValidatingWebhookConfiguration(r.kubeClient.AdmissionregistrationV1beta1(), cdWebhook)
	if err != nil {
		haLog.WithError(err).Error("error applying ClusterDeployment webhook")
		return reconcile.Result{}, err
	} else {
		haLog.WithField("changed", changed).Info("ClusterDeployment webhook updated")
	}

	haLog.Info("HiveAdmissionConfig components reconciled")

	return reconcile.Result{}, nil
}

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

// Add creates a new HiveAdmission Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveAdmission{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hiveadmission-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileHiveAdmission).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveAdmission).apiregClient, err = apiregclientv1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Watch for changes to HiveAdmission
	err = c.Watch(&source.Kind{Type: &hivev1alpha1.HiveAdmission{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create
	// Uncomment watch a Deployment created by HiveAdmission - change this for objects you create
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1alpha1.HiveAdmission{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileHiveAdmission{}

// ReconcileHiveAdmission reconciles a HiveAdmission object
type ReconcileHiveAdmission struct {
	client.Client
	scheme       *runtime.Scheme
	kubeClient   kubernetes.Interface
	apiregClient *apiregclientv1.ApiregistrationV1Client
}

// Reconcile reads that state of the cluster for a HiveAdmission object and makes changes based on the state read
// and what is in the HiveAdmission.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  The scaffolding writes
// a Deployment as an example
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// TODO: this lumps in with the hive controller rbac, should we manually manage our RBAC to separate the two?
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=hiveadmissions,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileHiveAdmission) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	haLog := log.WithField("controller", "hiveadmission")
	haLog.Info("Reconciling HiveAdmission components")

	// Fetch the HiveAdmission instance
	instance := &hivev1alpha1.HiveAdmission{}
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

	// Set owner refs on all objects in the deployment so deleting the operator CRD
	// will clean everything up:
	if err := controllerutil.SetControllerReference(instance, hiveAdmService, r.scheme); err != nil {
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
	return reconcile.Result{}, nil
}

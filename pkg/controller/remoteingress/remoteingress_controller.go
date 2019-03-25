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

package remoteingress

import (
	"context"
	"fmt"
	"reflect"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
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

	ingresscontroller "github.com/openshift/api/operator/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	controllerName = "remoteingress"

	remoteClusterIngressNamespace = "openshift-ingress-operator"

	remoteIngressKind = "IngressController"
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRemoteClusterIngress{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", controllerName),
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("remoteingress-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRemoteClusterIngress{}

// ReconcileRemoteClusterIngress reconciles the ingress objects defined in a ClusterDeployment object
type ReconcileRemoteClusterIngress struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and sets up
// any needed ClusterIngress objects up for syncing to the remote cluster.
//
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get
// +kubebuilder:rbac:groups=hive.openshift.io,resources=syncsets,verbs=get;create;update;delete;patch;list;watch
func (r *ReconcileRemoteClusterIngress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found (must have been deleted), return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	if cd.Spec.Ingress == nil {
		// the addmission controller will ensure that we get valid-looking
		// Spec.Ingress (ie no missing 'default', no going from a defined
		// ingress list to an empty list, etc)
		cdLog.Debug("no ingress objects defined. using default intaller behavior.")
		return reconcile.Result{}, nil
	}

	if err := r.syncClusterIngress(cd); err != nil {
		cdLog.Errorf("error syncing clusterIngress syncset: %v", err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRemoteClusterIngress) syncClusterIngress(cd *hivev1.ClusterDeployment) error {
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cdLog.Info("reconciling ClusterIngress for cluster deployment")

	rawList := rawExtensionsFromClusterDeployment(cd)

	return r.syncSyncSet(cd, rawList)
}

func rawExtensionsFromClusterDeployment(cd *hivev1.ClusterDeployment) []runtime.RawExtension {
	rawList := []runtime.RawExtension{}
	for _, ingress := range cd.Spec.Ingress {
		ingressObj := createIngressController(cd, ingress)
		raw := runtime.RawExtension{Object: ingressObj}
		rawList = append(rawList, raw)
	}

	return rawList
}

func newSyncSetSpec(cd *hivev1.ClusterDeployment, rawExtensions []runtime.RawExtension) *hivev1.SyncSetSpec {
	ssSpec := &hivev1.SyncSetSpec{
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Resources:         rawExtensions,
			ResourceApplyMode: "sync",
		},
		ClusterDeploymentRefs: []corev1.LocalObjectReference{
			{
				Name: cd.Name,
			},
		},
	}
	return ssSpec
}

func (r *ReconcileRemoteClusterIngress) syncSyncSet(cd *hivev1.ClusterDeployment, rawExtensions []runtime.RawExtension) error {

	ssName := fmt.Sprintf("%v-clusteringress", cd.Name)
	newSyncSetSpec := newSyncSetSpec(cd, rawExtensions)

	ss := &hivev1.SyncSet{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: ssName, Namespace: cd.Namespace}, ss)
	if errors.IsNotFound(err) {
		// create the syncset
		ss = &hivev1.SyncSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ssName,
				Namespace: cd.Namespace,
			},
			Spec: *newSyncSetSpec,
		}

		// ensure the syncset gets cleaned up when the clusterdeployment is deleted
		if err := controllerutil.SetControllerReference(cd, ss, r.scheme); err != nil {
			r.logger.WithError(err).Error("error setting owner reference")
			return err
		}

		if err := r.Create(context.TODO(), ss); err != nil {
			r.logger.Errorf("error creating sync set: %v", err)
			return err
		}

		return nil
	} else if err != nil {
		r.logger.Errorf("error checking for existing syncset: %v", err)
		return err
	}

	// update the syncset if there have been changes
	if !reflect.DeepEqual(ss.Spec, *newSyncSetSpec) {
		ss.Spec = *newSyncSetSpec
		if err := r.Update(context.TODO(), ss); err != nil {
			errDetails := fmt.Errorf("error updating existing syncset: %v", err)
			r.logger.Error(errDetails)
			return errDetails
		}
	}

	return nil
}

func createIngressController(cd *hivev1.ClusterDeployment, ingress hivev1.ClusterIngress) *ingresscontroller.IngressController {
	newIngress := ingresscontroller.IngressController{
		TypeMeta: metav1.TypeMeta{
			Kind:       remoteIngressKind,
			APIVersion: ingresscontroller.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingress.Name,
			Namespace: remoteClusterIngressNamespace,
		},
		Spec: ingresscontroller.IngressControllerSpec{
			Domain:            ingress.Domain,
			RouteSelector:     ingress.RouteSelector,
			NamespaceSelector: ingress.NamespaceSelector,
		},
	}

	return &newIngress
}

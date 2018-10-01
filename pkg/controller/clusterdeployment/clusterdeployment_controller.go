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

package clusterdeployment

import (
	"context"
	"reflect"

	log "github.com/sirupsen/logrus"

	kbatch "k8s.io/api/batch/v1"
	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/install"
)

const (
	installerImage   = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer"
	uninstallerImage = "registry.svc.ci.openshift.org/openshift/origin-v4.0:installer" // TODO
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeployment{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for jobs created by a ClusterDeployment:
	err = c.Watch(&source.Kind{Type: &kbatch.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeployment{}

// ReconcileClusterDeployment reconciles a ClusterDeployment object
type ReconcileClusterDeployment struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes based on the state read
// and what is in the ClusterDeployment.Spec
//
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
//
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileClusterDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cdLog.Info("reconciling cluster deployment")
	origCD := cd
	cd = cd.DeepCopy()

	job := install.GenerateInstallerJob(cd, installerImage, kapi.PullIfNotPresent)
	if err != nil {
		cdLog.Errorf("error generating install job", err)
		return reconcile.Result{}, err
	}

	if err := controllerutil.SetControllerReference(cd, job, r.scheme); err != nil {
		cdLog.Errorf("error setting controller reference on job", err)
		return reconcile.Result{}, err
	}

	if cd.DeletionTimestamp != nil {
		if !HasFinalizer(cd, hivev1.FinalizerDeprovision) {
			return reconcile.Result{}, nil
		}
		return r.syncDeletedClusterDeployment(cd, cdLog)
	}

	if !HasFinalizer(cd, hivev1.FinalizerDeprovision) {
		cdLog.Debugf("adding clusterdeployment finalizer")
		return reconcile.Result{}, r.addClusterDeploymentFinalizer(cd)
	}

	cdLog = cdLog.WithField("job", job.Name)

	// If the ClusterDeployment is already installed, we should stop
	// any further processing.
	if cd.Status.Installed {
		return reconcile.Result{}, nil
	}

	// Check if the Job already exists for this ClusterDeployment:
	existingJob := &kbatch.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: job.Name, Namespace: job.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Infof("creating job")
		err = r.Create(context.TODO(), job)
		if err != nil {
			cdLog.Errorf("error creating job: %v", err)
			return reconcile.Result{}, err
		}
	} else if err != nil {
		cdLog.Errorf("error getting job: %v", err)
		return reconcile.Result{}, err
	} else {
		// Job exists, check it's status:
		cdLog.Infof("conditions: %s", existingJob.Status.Conditions)
		cd.Status.Installed = isSuccessful(existingJob)
		cdLog.Infof("successful: %s", cd.Status.Installed)
	}

	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment")
		err = r.Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment: %v", err)
			return reconcile.Result{}, err
		}
	} else {
		cdLog.Infof("cluster deployment status unchanged")
	}

	cdLog.Debugf("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) syncDeletedClusterDeployment(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	// Generate an uninstall job:
	uninstallJob, cm, err := install.GenerateUninstallerJob(cd, installerImage, kapi.PullIfNotPresent)
	if err != nil {
		cdLog.Errorf("error generating uninstaller job: %v", err)
		return reconcile.Result{}, err
	}

	err = controllerutil.SetControllerReference(cd, uninstallJob, r.scheme)
	if err != nil {
		cdLog.Errorf("error setting controller reference on job: %v", err)
		return reconcile.Result{}, err
	}

	// Check if uninstall job already exists:
	existingJob := &kbatch.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: uninstallJob.Name, Namespace: uninstallJob.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Infof("creating configmap for uninstall job")
		err = r.Create(context.TODO(), cm)
		if err != nil {
			cdLog.Errorf("error creating configmap for uninstall job: %v", err)
			return reconcile.Result{}, err
		}
		err = r.Create(context.TODO(), uninstallJob)
		if err != nil {
			cdLog.Errorf("error creating uninstall job: %v", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		cdLog.Errorf("error getting uninstall job: %v", err)
		return reconcile.Result{}, err
	}

	// Uninstall job exists, check it's status and if successful, remove the finalizer:
	if isSuccessful(existingJob) {
		cdLog.Infof("uninstall job successful, removing finalizer")
		return reconcile.Result{}, r.removeClusterDeploymentFinalizer(cd)
	}

	cdLog.Infof("uninstall job not yet successful")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterDeployment) addClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	AddFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

func (r *ReconcileClusterDeployment) removeClusterDeploymentFinalizer(cd *hivev1.ClusterDeployment) error {
	cd = cd.DeepCopy()
	DeleteFinalizer(cd, hivev1.FinalizerDeprovision)
	return r.Update(context.TODO(), cd)
}

// getJobConditionStatus gets the status of the condition in the job. If the
// condition is not found in the job, then returns False.
func getJobConditionStatus(job *kbatch.Job, conditionType kbatch.JobConditionType) kapi.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return kapi.ConditionFalse
}

func isSuccessful(job *kbatch.Job) bool {
	return getJobConditionStatus(job, kbatch.JobComplete) == kapi.ConditionTrue
}

func isFailed(job *kbatch.Job) bool {
	return getJobConditionStatus(job, kbatch.JobFailed) == kapi.ConditionTrue
}

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

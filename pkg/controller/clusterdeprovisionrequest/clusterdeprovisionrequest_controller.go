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

package clusterdeprovisionrequest

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/images"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	controllerName = "clusterDeprovisionRequest"
)

var (
	metricUninstallJobDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_uninstall_job_duration_seconds",
			Help:    "Distribution of the runtime of completed uninstall jobs.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
	)
)

func init() {
	metrics.Registry.MustRegister(metricUninstallJobDuration)
}

// Add creates a new ClusterDeprovisionRequest Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeprovisionRequest{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeprovisionrequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeprovisionRequest
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeprovisionRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for uninstall jobs created for ClusterDeprovisionRequests
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeprovisionRequest{},
	})

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeprovisionRequest{}

// ReconcileClusterDeprovisionRequest reconciles a ClusterDeprovisionRequest object
type ReconcileClusterDeprovisionRequest struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeprovisionRequest object and makes changes based on the state read
// and what is in the ClusterDeprovisionRequest.Spec
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeprovisionrequests;clusterdeprovisionrequests/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeprovisionrequests/status,verbs=get;update;patch
func (r *ReconcileClusterDeprovisionRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	rLog := log.WithFields(log.Fields{
		"name":       request.NamespacedName.String(),
		"controller": controllerName,
	})
	// Fetch the ClusterDeprovisionRequest instance
	instance := &hivev1.ClusterDeprovisionRequest{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			rLog.Debug("clusterdeprovisionrequest not found, skipping")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		rLog.WithError(err).Error("cannot get clusterdeprovisionrequest")
		return reconcile.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		rLog.Debug("clusterdeprovisionrequest being deleted, skipping")
		return reconcile.Result{}, nil
	}

	if instance.Status.Completed {
		rLog.Debug("clusterdeprovisionrequest is complete, skipping")
		return reconcile.Result{}, nil
	}

	// Generate an uninstall job
	hiveImage := images.GetHiveImage(rLog)
	rLog.Debug("generating uninstall job")
	uninstallJob, err := install.GenerateUninstallerJobForDeprovisionRequest(instance, hiveImage)
	if err != nil {
		rLog.Errorf("error generating uninstaller job: %v", err)
		return reconcile.Result{}, err
	}

	rLog.Debug("setting uninstall job controller reference")
	err = controllerutil.SetControllerReference(instance, uninstallJob, r.scheme)
	if err != nil {
		rLog.Errorf("error setting controller reference on job: %v", err)
		return reconcile.Result{}, err
	}

	// Check if uninstall job already exists:
	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: uninstallJob.Name, Namespace: uninstallJob.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		_, err := controllerutils.SetupClusterInstallServiceAccount(r, instance.Namespace, rLog)
		if err != nil {
			rLog.WithError(err).Error("error setting up service account and role")
			return reconcile.Result{}, err
		}
		rLog.Debug("uninstall job does not exist, creating it")
		err = r.Create(context.TODO(), uninstallJob)
		if err != nil {
			rLog.WithError(err).Errorf("error creating uninstall job")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	} else if err != nil {
		rLog.WithError(err).Errorf("error getting uninstall job")
		return reconcile.Result{}, err
	}
	rLog.Debug("uninstall job exists, checking its status")

	// Uninstall job exists, check its status and if successful, set the deprovision request status to complete
	if controllerutils.IsSuccessful(existingJob) {
		rLog.Infof("uninstall job successful, setting completed status")
		// jobDuration calculates the time elapsed since the uninstall job started for deprovision job
		jobDuration := existingJob.Status.CompletionTime.Time.Sub(existingJob.Status.StartTime.Time)
		rLog.WithField("duration", jobDuration.Seconds()).Debug("uninstall job completed")
		instance.Status.Completed = true
		err = r.Status().Update(context.TODO(), instance)
		if err != nil {
			rLog.WithError(err).Error("error updating request status")
			return reconcile.Result{}, err
		}
		metricUninstallJobDuration.Observe(float64(jobDuration.Seconds()))
		return reconcile.Result{}, nil
	}
	rLog.Infof("uninstall job not yet successful")
	return reconcile.Result{}, nil
}

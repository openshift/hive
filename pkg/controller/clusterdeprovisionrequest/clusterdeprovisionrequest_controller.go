package clusterdeprovisionrequest

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	controllerName    = "clusterDeprovisionRequest"
	jobHashAnnotation = "hive.openshift.io/jobhash"
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
	return &ReconcileClusterDeprovisionRequest{Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeprovisionrequest-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error getting new clusterdeprovisionrequest-controller")
		return err
	}

	// Watch for changes to ClusterDeprovisionRequest
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeprovisionRequest{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching changes to clusterdeprovisionrequest")
		return err
	}

	// Watch for uninstall jobs created for ClusterDeprovisionRequests
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeprovisionRequest{},
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching  uninstall jobs created for clusterdeprovisionreques")
		return err
	}

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
func (r *ReconcileClusterDeprovisionRequest) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	rLog := log.WithFields(log.Fields{
		"name":       request.NamespacedName.String(),
		"controller": controllerName,
	})
	// For logging, we need to see when the reconciliation loop starts and ends.
	rLog.Info("reconciling cluster deprovision request")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		rLog.WithField("elapsed", dur).Info("reconcile complete")
	}()
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
	rLog.Debug("generating uninstall job")
	uninstallJob, err := install.GenerateUninstallerJobForDeprovisionRequest(instance)
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

	jobHash, err := controllerutils.CalculateJobSpecHash(uninstallJob)
	if err != nil {
		rLog.WithError(err).Error("failed to calculate hash for generated deprovision job")
		return reconcile.Result{}, err
	}
	if uninstallJob.Annotations == nil {
		uninstallJob.Annotations = map[string]string{}
	}
	uninstallJob.Annotations[jobHashAnnotation] = jobHash

	// Check if uninstall job already exists:
	existingJob := &batchv1.Job{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: uninstallJob.Name, Namespace: uninstallJob.Namespace}, existingJob)
	if err != nil && errors.IsNotFound(err) {
		rLog.Debug("uninstall job does not exist, creating it")
		err = r.Create(context.TODO(), uninstallJob)
		if err != nil {
			rLog.WithError(err).Log(controllerutils.LogLevel(err), "error creating uninstall job")
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
			rLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating request status")
			return reconcile.Result{}, err
		}
		metricUninstallJobDuration.Observe(float64(jobDuration.Seconds()))
		return reconcile.Result{}, nil
	}

	// Check if the job should be regenerated:
	newJobNeeded := false
	if existingJob.Annotations == nil {
		newJobNeeded = true
	} else if _, ok := existingJob.Annotations[jobHashAnnotation]; !ok {
		// this job predates tracking the job hash, so assume we need a new job
		newJobNeeded = true
	} else if existingJob.Annotations[jobHashAnnotation] != jobHash {
		// delete the job so we get a fresh one with the new job spec
		newJobNeeded = true
	}

	if newJobNeeded {
		if existingJob.DeletionTimestamp == nil {
			rLog.Info("deleting existing deprovision job due to updated/missing hash detected")
			err := r.Delete(context.TODO(), existingJob, client.PropagationPolicy(metav1.DeletePropagationForeground))
			if err != nil {
				rLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting outdated deprovision job")
			}
		}
		return reconcile.Result{}, err
	}

	rLog.Infof("uninstall job not yet successful")
	return reconcile.Result{}, nil
}

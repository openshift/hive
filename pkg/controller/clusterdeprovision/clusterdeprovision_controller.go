package clusterdeprovision

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	controllerName    = "clusterDeprovision"
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

// Add creates a new ClusterDeprovision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterDeprovision{Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterdeprovision-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error getting new clusterdeprovision-controller")
		return err
	}

	// Watch for changes to ClusterDeprovision
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeprovision{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching changes to clusterdeprovision")
		return err
	}

	// Watch for uninstall jobs created for ClusterDeprovisions
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterDeprovision{},
	})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching  uninstall jobs created for clusterdeprovisionreques")
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeprovision{}

// ReconcileClusterDeprovision reconciles a ClusterDeprovision object
type ReconcileClusterDeprovision struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterDeprovision object and makes changes based on the state read
// and what is in the ClusterDeprovision.Spec
func (r *ReconcileClusterDeprovision) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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
	// Fetch the ClusterDeprovision instance
	instance := &hivev1.ClusterDeprovision{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			rLog.Debug("clusterdeprovision not found, skipping")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		rLog.WithError(err).Error("cannot get clusterdeprovision")
		return reconcile.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		rLog.Debug("clusterdeprovision being deleted, skipping")
		return reconcile.Result{}, nil
	}

	if instance.Status.Completed {
		rLog.Debug("clusterdeprovision is complete, skipping")
		return reconcile.Result{}, nil
	}

	// Check if there is a ClusterDeployment owning this Deprovision, if so look it up and
	// make sure it has a deletion timestamp. Otherwise bail out as a safety check.
	oRef := metav1.GetControllerOf(instance)
	if oRef == nil {
		// TODO: this was once supported to cleanup self-managed "preserveOnDelete" clusters,
		// but the feature was killed off. For now we'd rather not open the door to dangling
		// ClusterDeprovisions with no associated cluster.
		rLog.Warn("ClusterDeprovision does not have an owning ClusterDeployment")
		return reconcile.Result{}, nil
	}
	if oRef.Kind != "ClusterDeployment" || !strings.HasPrefix(oRef.APIVersion, "hive.openshift.io") {
		rLog.Warnf("ClusterDeprovision has a non-ClusterDeployment owner: %v", oRef)
		return reconcile.Result{}, nil
	}
	cd := &hivev1.ClusterDeployment{}
	if err := r.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace, Name: oRef.Name}, cd); err != nil {
		rLog.Error("error looking up ClusterDeployment that owns ClusterDeprovision")
		return reconcile.Result{}, fmt.Errorf("error looking up ClusterDeployment that owns ClusterDeprovision")
	}
	if cd.DeletionTimestamp == nil {
		rLog.Error("ClusterDeprovision created for ClusterDeployment that has not been deleted")
		return reconcile.Result{}, nil
	}

	// Generate an uninstall job
	rLog.Debug("generating uninstall job")
	uninstallJob, err := install.GenerateUninstallerJobForDeprovision(instance)
	if err != nil {
		rLog.Errorf("error generating uninstaller job: %v", err)
		return reconcile.Result{}, err
	}

	rLog.Debug("setting uninstall job controller reference")
	rLog.WithField("derivedObject", uninstallJob.Name).Debug("Setting labels on derived object")
	uninstallJob.Labels = k8slabels.AddLabel(uninstallJob.Labels, constants.ClusterDeprovisionNameLabel, instance.Name)
	uninstallJob.Labels = k8slabels.AddLabel(uninstallJob.Labels, constants.JobTypeLabel, constants.JobTypeDeprovision)
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

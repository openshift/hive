package clusterprovision

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	controllerName = "clusterProvision"

	// clusterProvisionLabelKey is the label that is used to identify
	// resources descendant from a cluster provision.
	clusterProvisionLabelKey = "hive.openshift.io/cluster-provision"
)

var (
	metricInstallErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_install_errors",
		Help: "Counter incremented every time we observe certain errors strings in install logs.",
	},
		[]string{"cluster_type", "reason"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricInstallErrors)
}

// Add creates a new ClusterProvision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterProvision{Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterprovision-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterProvision
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterProvision{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for install jobs created for ClusterProvision
	err = c.Watch(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.ClusterProvision{},
	})

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterProvision{}

// ReconcileClusterProvision reconciles a ClusterProvision object
type ReconcileClusterProvision struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterProvision object and makes changes based on the state read
// and what is in the ClusterProvision.Spec
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterprovisions;clusterprovisions/status;clusterprovisions/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileClusterProvision) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	pLog := log.WithFields(log.Fields{
		"name":       request.NamespacedName.String(),
		"controller": controllerName,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	pLog.Info("reconciling cluster provision")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		pLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterProvision instance
	instance := &hivev1.ClusterProvision{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			pLog.Debug("ClusterProvision not found, skipping")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		pLog.WithError(err).Error("cannot get ClusterProvision")
		return reconcile.Result{}, err
	}

	if !instance.DeletionTimestamp.IsZero() {
		pLog.Debug("ClusterProvision being deleted, skipping")
		return reconcile.Result{}, nil
	}

	switch instance.Spec.Stage {
	case hivev1.ClusterProvisionStageComplete, hivev1.ClusterProvisionStageFailed:
		pLog.Debugf("ClusterProvision is %s. Nothing more to do", instance.Spec.Stage)
		return reconcile.Result{}, nil
	}

	if instance.Status.Job != nil {
		return r.reconcileRunningJob(instance, pLog)
	}

	return r.reconcileNewProvision(instance, pLog)
}

func (r *ReconcileClusterProvision) reconcileNewProvision(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	existingJobs, err := r.existingJobs(instance, pLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch len(existingJobs) {
	case 0:
		return r.createJob(instance, pLog)
	case 1:
		return r.adoptJob(instance, existingJobs[0], pLog)
	default:
		return r.abortProvision(instance, "TooManyJobs", "more than one install job exists", pLog)
	}
}

func (r *ReconcileClusterProvision) createJob(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	job, err := install.GenerateInstallerJob(instance)
	if err != nil {
		pLog.WithError(err).Error("error generating install job")
		return reconcile.Result{}, err
	}
	job.Labels[clusterProvisionLabelKey] = instance.Name

	pLog = pLog.WithField("job", job.Name)

	if err = controllerutil.SetControllerReference(instance, job, r.scheme); err != nil {
		pLog.WithError(err).Error("error setting controller reference on job")
		return reconcile.Result{}, err
	}

	pLog.Infof("creating install job")
	if err := r.Create(context.TODO(), job); err != nil {
		pLog.WithError(err).Error("error creating job")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) adoptJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	instance.Status.Job = &corev1.LocalObjectReference{Name: job.Name}
	instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
		instance.Status.Conditions,
		hivev1.ClusterProvisionJobCreated,
		corev1.ConditionTrue,
		"JobCreated",
		"Install job has been created",
		controllerutils.UpdateConditionAlways,
	)
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update status conditions")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// check if the job has completed
func (r *ReconcileClusterProvision) reconcileRunningJob(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Debug("reconciling running job")

	job := &batchv1.Job{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Status.Job.Name, Namespace: instance.Namespace}, job); {
	case apierrors.IsNotFound(err):
		if cond := controllerutils.FindClusterProvisionCondition(instance.Status.Conditions, hivev1.ClusterProvisionFailedCondition); cond == nil {
			pLog.Error("install job lost")
			instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
				instance.Status.Conditions,
				hivev1.ClusterProvisionFailedCondition,
				corev1.ConditionTrue,
				"JobNotFound",
				"install job not found",
				controllerutils.UpdateConditionAlways,
			)
		} else {
			pLog.Info("install job from aborted provision has been deleted")
		}
		instance.Spec.Stage = hivev1.ClusterProvisionStageFailed
		if err := r.Update(context.TODO(), instance); err != nil {
			pLog.WithError(err).Error("cannot update provision stage")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case err != nil:
		pLog.WithError(err).Error("could not get install job")
		return reconcile.Result{}, err
	}

	pLog = pLog.WithField("job", job.Name)

	stage := hivev1.ClusterProvisionStage("")
	cond := hivev1.ClusterProvisionConditionType("")
	reason := ""
	message := ""
	switch {
	case controllerutils.IsSuccessful(job):
		pLog.Info("install job succeeded")
		stage = hivev1.ClusterProvisionStageComplete
		cond = hivev1.ClusterProvisionCompletedCondition
		reason = "InstallComplete"
		message = "Install job has completed successfully"
	case controllerutils.IsFailed(job):
		pLog.Info("install job failed")
		stage = hivev1.ClusterProvisionStageFailed
		cond = hivev1.ClusterProvisionFailedCondition
		reason = "JobFailed"
		message = "Install job failed"
		// Increment a counter metric for this cluster type and error reason:
		metricInstallErrors.WithLabelValues(hivemetrics.GetClusterDeploymentType(instance), reason).Inc()
	default:
		pLog.Debug("install job still running")
		return reconcile.Result{}, nil
	}

	instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
		instance.Status.Conditions,
		cond,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionAlways,
	)
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update status conditions")
		return reconcile.Result{}, err
	}
	instance.Spec.Stage = stage
	if err := r.Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update provision stage")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) abortProvision(instance *hivev1.ClusterProvision, reason string, message string, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Infof("aborting provision (%s): %s", reason, message)
	instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
		instance.Status.Conditions,
		hivev1.ClusterProvisionFailedCondition,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionAlways,
	)
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update status conditions")
		return reconcile.Result{}, err
	}
	if instance.Status.Job != nil {
		job := &batchv1.Job{}
		switch err := r.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Status.Job.Name}, job); {
		case apierrors.IsNotFound(err):
			pLog.Warn("install job for aborted provision already gone before it was deleted")
			return reconcile.Result{}, nil
		case err != nil:
			pLog.WithError(err).Error("could not get install job")
			return reconcile.Result{}, err
		}
		if err := r.Delete(context.TODO(), job, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
			pLog.WithError(err).Error("could not delete install job")
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) existingJobs(provision *hivev1.ClusterProvision, pLog log.FieldLogger) ([]*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	if err := r.List(
		context.TODO(),
		jobList,
		client.InNamespace(provision.Namespace),
		client.MatchingLabels(map[string]string{clusterProvisionLabelKey: provision.Name}),
	); err != nil {
		pLog.WithError(err).Warn("could not list jobs for clusterprovision")
		return nil, errors.Wrap(err, "could not list jobs")
	}
	jobs := make([]*batchv1.Job, len(jobList.Items))
	for i := range jobList.Items {
		jobs[i] = &jobList.Items[i]
	}
	return jobs, nil
}

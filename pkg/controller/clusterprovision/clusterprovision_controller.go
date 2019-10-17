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
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterProvision")

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
	logger := log.WithField("controller", controllerName)
	return &ReconcileClusterProvision{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:       mgr.GetScheme(),
		logger:       logger,
		expectations: controllerutils.NewExpectations(logger),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	provisionReconciler, ok := r.(*ReconcileClusterProvision)
	if !ok {
		return errors.New("reconciler supplied is not a ReconcileClusterProvision")
	}

	// Create a new controller
	c, err := controller.New("clusterprovision-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return errors.Wrap(err, "could not create controller")
	}

	// Watch for changes to ClusterProvision
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterProvision{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return errors.Wrap(err, "could not watch clusterprovisions")
	}

	// Watch for install jobs created for ClusterProvision
	if err := provisionReconciler.watchJobs(c); err != nil {
		return errors.Wrap(err, "could not watch jobs")
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(clusterDeploymentWatchHandler),
	}); err != nil {
		return errors.Wrap(err, "could not watch clusterdeployments")
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterProvision{}

// ReconcileClusterProvision reconciles a ClusterProvision object
type ReconcileClusterProvision struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger
	// A TTLCache of job creates each clusterprovision expects to see
	expectations controllerutils.ExpectationsInterface
}

// Reconcile reads that state of the cluster for a ClusterProvision object and makes changes based on the state read
// and what is in the ClusterProvision.Spec
func (r *ReconcileClusterProvision) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	pLog := r.logger.WithField("name", request.NamespacedName)

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
			r.expectations.DeleteExpectations(request.NamespacedName.String())
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

	if !r.expectations.SatisfiedExpectations(request.String()) {
		pLog.Debug("waiting for expectations to be satisfied")
		return reconcile.Result{}, nil
	}

	switch instance.Spec.Stage {
	case hivev1.ClusterProvisionStageInitializing:
		if instance.Status.Job != nil {
			return r.reconcileRunningJob(instance, pLog)
		}
		return r.reconcileNewProvision(instance, pLog)
	case hivev1.ClusterProvisionStageProvisioning:
		if instance.Status.Job != nil {
			return r.reconcileRunningJob(instance, pLog)
		}
		return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, "NoJobReference", "Missing reference to install job", pLog)
	case hivev1.ClusterProvisionStageComplete, hivev1.ClusterProvisionStageFailed:
		pLog.Debugf("ClusterProvision is %s. Nothing more to do", instance.Spec.Stage)
		return reconcile.Result{}, nil
	default:
		pLog.Errorf("ClusterProvision has unknown stage %q", instance.Spec.Stage)
		return reconcile.Result{}, nil
	}
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
	r.expectations.ExpectCreations(types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}.String(), 1)
	if err := r.Create(context.TODO(), job); err != nil {
		pLog.WithError(err).Error("error creating job")
		r.expectations.CreationObserved(types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}.String())
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) adoptJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	instance.Status.Job = &corev1.LocalObjectReference{Name: job.Name}
	return reconcile.Result{}, r.setCondition(instance, hivev1.ClusterProvisionJobCreated, "JobCreated", "Install job has been created", pLog)
}

// check if the job has completed
func (r *ReconcileClusterProvision) reconcileRunningJob(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Debug("reconciling running job")

	job := &batchv1.Job{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Status.Job.Name, Namespace: instance.Namespace}, job); {
	case apierrors.IsNotFound(err):
		if cond := controllerutils.FindClusterProvisionCondition(instance.Status.Conditions, hivev1.ClusterProvisionFailedCondition); cond == nil {
			pLog.Error("install job lost")
			if err := r.setCondition(instance, hivev1.ClusterProvisionFailedCondition, "JobNotFound", "install job not found", pLog); err != nil {
				return reconcile.Result{}, err
			}
		} else {
			pLog.Info("install job from aborted provision has been deleted")
		}
		return reconcile.Result{}, r.setStage(instance, hivev1.ClusterProvisionStageFailed, pLog)
	case err != nil:
		pLog.WithError(err).Error("could not get install job")
		return reconcile.Result{}, err
	}

	pLog = pLog.WithField("job", job.Name)

	switch {
	case controllerutils.IsSuccessful(job):
		if instance.Spec.Stage == hivev1.ClusterProvisionStageInitializing {
			pLog.Error("install job completed without completing initialization")
			return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, "InitializationNotComplete", "Install job completed without completing initialization", pLog)
		}
		return r.reconcileSuccessfulJob(instance, job, pLog)
	case controllerutils.IsFailed(job):
		return r.reconcileFailedJob(instance, job, pLog)
	}

	pLog.Debug("install job still running")

	if instance.Spec.Stage == hivev1.ClusterProvisionStageInitializing && instance.Spec.InfraID != nil {
		cd := hivev1.ClusterDeployment{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      instance.Spec.ClusterDeployment.Name,
				Namespace: instance.Namespace,
			},
			&cd,
		); err != nil {
			pLog.WithError(err).Error("could not get clusterdeployment")
			return reconcile.Result{}, err
		}
		if cd.Status.InfraID == *instance.Spec.InfraID {
			return r.startProvisioning(instance, pLog)
		}
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) reconcileSuccessfulJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("install job succeeded")
	return r.transitionStage(instance, hivev1.ClusterProvisionStageComplete, "InstallComplete", "Install job has completed successfully", pLog)
}

func (r *ReconcileClusterProvision) reconcileFailedJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("install job failed")
	reason, message := r.parseInstallLog(instance.Spec.InstallLog, pLog)
	// Increment a counter metric for this cluster type and error reason:
	metricInstallErrors.WithLabelValues(hivemetrics.GetClusterDeploymentType(instance), reason).Inc()
	return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, reason, message, pLog)
}

func (r *ReconcileClusterProvision) startProvisioning(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("provision initialization complete")
	return r.transitionStage(instance, hivev1.ClusterProvisionStageProvisioning, "InitializationComplete", "Install job has completed its initialization. Provisioning started.", pLog)
}

func (r *ReconcileClusterProvision) abortProvision(instance *hivev1.ClusterProvision, reason string, message string, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Infof("aborting provision (%s): %s", reason, message)
	if instance.Status.Job == nil {
		return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, reason, message, pLog)
	}
	if err := r.setCondition(instance, hivev1.ClusterProvisionFailedCondition, reason, message, pLog); err != nil {
		return reconcile.Result{}, err
	}
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
		pLog.WithError(err).Log(controllerutils.LogLevel(err), "could not delete install job")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) transitionStage(
	instance *hivev1.ClusterProvision,
	stage hivev1.ClusterProvisionStage,
	reason string,
	message string,
	pLog log.FieldLogger,
) (reconcile.Result, error) {
	pLog.Infof("transitioning to %s", stage)
	var conditionType hivev1.ClusterProvisionConditionType
	switch stage {
	case hivev1.ClusterProvisionStageInitializing:
		return reconcile.Result{}, errors.New("cannot transition to initializing")
	case hivev1.ClusterProvisionStageProvisioning:
		conditionType = hivev1.ClusterProvisionInitializedCondition
	case hivev1.ClusterProvisionStageComplete:
		conditionType = hivev1.ClusterProvisionCompletedCondition
	case hivev1.ClusterProvisionStageFailed:
		conditionType = hivev1.ClusterProvisionFailedCondition
	default:
		return reconcile.Result{}, errors.New("unknown stage")
	}
	if err := r.setCondition(instance, conditionType, reason, message, pLog); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.setStage(instance, stage, pLog); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) setCondition(
	instance *hivev1.ClusterProvision,
	conditionType hivev1.ClusterProvisionConditionType,
	reason string,
	message string,
	pLog log.FieldLogger,
) error {
	instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
		instance.Status.Conditions,
		conditionType,
		corev1.ConditionTrue,
		reason,
		message,
		controllerutils.UpdateConditionAlways,
	)
	if err := r.Status().Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update status conditions")
		return err
	}
	return nil
}

func (r *ReconcileClusterProvision) setStage(instance *hivev1.ClusterProvision, stage hivev1.ClusterProvisionStage, pLog log.FieldLogger) error {
	instance.Spec.Stage = stage
	if err := r.Update(context.TODO(), instance); err != nil {
		pLog.WithError(err).Error("cannot update provision stage")
		return err
	}
	return nil
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

func clusterDeploymentWatchHandler(a handler.MapObject) []reconcile.Request {
	cd := a.Object.(*hivev1.ClusterDeployment)
	if cd == nil {
		// Wasn't a ClusterDeployment, bail out. This should not happen.
		log.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a.Object)
		return nil
	}

	if cd.Status.Provision == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      cd.Status.Provision.Name,
				Namespace: cd.Namespace,
			},
		},
	}
}

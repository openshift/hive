package clusterprovision

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
	installertypes "github.com/openshift/installer/pkg/types"
)

const (
	ControllerName = hivev1.ClusterProvisionControllerName

	// clusterProvisionLabelKey is the label that is used to identify
	// resources descendant from a cluster provision.
	clusterProvisionLabelKey = "hive.openshift.io/cluster-provision"

	resultSuccess = "success"
	resultFailure = "failure"

	podStatusCheckDelay = 60 * time.Second
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("ClusterProvision")
)

// Add creates a new ClusterProvision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	// Read the metrics config from hiveconfig and set values for mapClusterTypeLabelToValue, if present
	mConfig, err := hivemetrics.ReadMetricsConfig()
	if err != nil {
		log.WithError(err).Error("error reading metrics config")
		return err
	}
	// Register the metrics. This is done here to ensure we define the metrics with optional label support after we have
	// read the hiveconfig, and we register them only once.
	registerMetrics(mConfig, logger)

	return add(mgr, newReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	logger := log.WithField("controller", ControllerName)
	return &ReconcileClusterProvision{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:       mgr.GetScheme(),
		logger:       logger,
		expectations: controllerutils.NewExpectations(logger),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	provisionReconciler, ok := r.(*ReconcileClusterProvision)
	if !ok {
		return errors.New("reconciler supplied is not a ReconcileClusterProvision")
	}

	// Create a new controller
	c, err := controller.New("clusterprovision-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(provisionReconciler, provisionReconciler.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return errors.Wrap(err, "could not create controller")
	}

	// Watch for changes to ClusterProvision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterProvision{}), &handler.EnqueueRequestForObject{}); err != nil {
		return errors.Wrap(err, "could not watch clusterprovisions")
	}

	// Watch for install jobs created for ClusterProvision
	if err := provisionReconciler.watchJobs(mgr, c); err != nil {
		return errors.Wrap(err, "could not watch jobs")
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), handler.EnqueueRequestsFromMapFunc(clusterDeploymentWatchHandler)); err != nil {
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
func (r *ReconcileClusterProvision) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	pLog := controllerutils.BuildControllerLogger(ControllerName, "clusterProvision", request.NamespacedName)
	pLog.Info("reconciling cluster provision")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, pLog)
	defer recobsrv.ObserveControllerReconcileTime()

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
	pLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: instance}, pLog)

	// Ensure owner references are correctly set
	err = controllerutils.ReconcileOwnerReferences(instance, generateOwnershipUniqueKeys(instance), r, r.scheme, pLog)
	if err != nil {
		pLog.WithError(err).Error("Error reconciling object ownership")
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
		if instance.Status.JobRef != nil {
			return r.reconcileRunningJob(instance, pLog)
		}
		return r.reconcileNewProvision(instance, pLog)
	case hivev1.ClusterProvisionStageProvisioning:
		if instance.Status.JobRef != nil {
			return r.reconcileRunningJob(instance, pLog)
		}
		return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, "NoJobReference", "Missing reference to install job", pLog)
	case hivev1.ClusterProvisionStageComplete:
		pLog.Debugf("ClusterProvision is %s", instance.Spec.Stage)
		if instance.Status.JobRef != nil && time.Since(instance.CreationTimestamp.Time) > (24*time.Hour) {
			return r.deleteInstallJob(instance, pLog)
		}
		// installJobDeletionRecheckDelay will be duration between current time and expected install job deletion time (provision creation time + 24 hours)
		installJobDeletionRecheckDelay := time.Until(instance.CreationTimestamp.Time.Add(24 * time.Hour))
		return reconcile.Result{RequeueAfter: installJobDeletionRecheckDelay}, nil
	case hivev1.ClusterProvisionStageFailed:
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

	pLog.WithField("derivedObject", job.Name).Debug("Setting labels on derived object")
	job.Labels = k8slabels.AddLabel(job.Labels, constants.ClusterProvisionNameLabel, instance.Name)
	job.Labels = k8slabels.AddLabel(job.Labels, constants.JobTypeLabel, constants.JobTypeProvision)
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
	instance.Status.JobRef = &corev1.LocalObjectReference{Name: job.Name}
	return reconcile.Result{}, r.setCondition(instance, hivev1.ClusterProvisionJobCreated, corev1.ConditionTrue, "JobCreated", "Install job has been created", controllerutils.UpdateConditionAlways, pLog)
}

// check if the job has completed
func (r *ReconcileClusterProvision) reconcileRunningJob(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Debug("reconciling running job")

	job := &batchv1.Job{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: instance.Status.JobRef.Name, Namespace: instance.Namespace}, job); {
	case apierrors.IsNotFound(err):
		if cond := controllerutils.FindCondition(instance.Status.Conditions, hivev1.ClusterProvisionFailedCondition); cond == nil {
			pLog.Error("install job lost")
			if err := r.setCondition(instance, hivev1.ClusterProvisionFailedCondition, corev1.ConditionTrue, "JobNotFound", "install job not found", controllerutils.UpdateConditionAlways, pLog); err != nil {
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

	if time.Since(job.CreationTimestamp.Time) > podStatusCheckDelay {
		installPod, err := r.getInstallPod(job, pLog)
		if err != nil {
			pLog.WithError(err).Error("could not get install pod")
			if err := r.setCondition(instance, hivev1.InstallPodStuckCondition, corev1.ConditionTrue, "InstallPodMissing", err.Error(), controllerutils.UpdateConditionIfReasonOrMessageChange, pLog); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}

		if installPod.Status.Phase == "Pending" {
			pLog.WithField("pod", installPod.Name).Error("install pod is stuck")
			if err := r.setCondition(instance, hivev1.InstallPodStuckCondition, corev1.ConditionTrue, "PodInPendingPhase", "pod is in pending phase", controllerutils.UpdateConditionIfReasonOrMessageChange, pLog); err != nil {
				return reconcile.Result{}, err
			}
			// Since this controller is not watching pods, the ClusterProvision will not be re-synced if the pod does
			// transition to the running phase later. However, if the pod does start running, then soon after either the
			// install manager will set the InfraID on the ClusterProvision or the pod will fail.
			return reconcile.Result{}, nil
		}
		if cond := controllerutils.FindCondition(instance.Status.Conditions, hivev1.InstallPodStuckCondition); cond != nil && cond.Status == corev1.ConditionTrue {
			if err := r.setCondition(instance, hivev1.InstallPodStuckCondition, corev1.ConditionFalse, "PodInRunningPhase", "pod is in running phase", controllerutils.UpdateConditionNever, pLog); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	if instance.Spec.Stage == hivev1.ClusterProvisionStageInitializing && instance.Spec.InfraID != nil {
		cd := hivev1.ClusterDeployment{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      instance.Spec.ClusterDeploymentRef.Name,
				Namespace: instance.Namespace,
			},
			&cd,
		); err != nil {
			pLog.WithError(err).Error("could not get clusterdeployment")
			return reconcile.Result{}, err
		}
		if cd.Spec.ClusterMetadata != nil && cd.Spec.ClusterMetadata.InfraID == *instance.Spec.InfraID {
			return r.startProvisioning(instance, pLog)
		}
	}

	if timeUntilNextPodStatusCheck := podStatusCheckDelay - time.Since(job.CreationTimestamp.Time); timeUntilNextPodStatusCheck > 0 {
		return reconcile.Result{RequeueAfter: timeUntilNextPodStatusCheck}, nil
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) getInstallPod(job *batchv1.Job, pLog log.FieldLogger) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	podLabelSelector, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		pLog.WithError(err).Error("could not create pod selector from job")
		return nil, fmt.Errorf("could not create pod selector from job")
	}
	if err := r.List(
		context.TODO(),
		podList,
		client.MatchingLabelsSelector{Selector: podLabelSelector},
		client.InNamespace(job.Namespace),
	); err != nil {
		pLog.WithError(err).Error("could not list install pods")
		return nil, fmt.Errorf("could not list install pods")
	}

	switch len(podList.Items) {
	case 0:
		pLog.Error("install pod not found")
		return nil, fmt.Errorf("install pod not found")
	case 1:
		return &podList.Items[0], nil
	default:
		pLog.Error("more than one install pod exists")
		return nil, fmt.Errorf("more than one install pod exists")
	}
}

func (r *ReconcileClusterProvision) reconcileSuccessfulJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("install job succeeded")
	result, err := r.transitionStage(instance, hivev1.ClusterProvisionStageComplete, "InstallComplete", "Install job has completed successfully", pLog)
	if err == nil {
		metricClusterProvisionsTotal.Observe(instance, map[string]string{"result": resultSuccess}, 1)
	}
	return result, err
}

func (r *ReconcileClusterProvision) reconcileFailedJob(instance *hivev1.ClusterProvision, job *batchv1.Job, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("install job failed")
	reason, message := r.parseInstallLog(instance.Spec.InstallLog, pLog)
	if controllerutils.IsDeadlineExceeded(job) && reason == unknownReason {
		reason, message = "AttemptDeadlineExceeded", "Install job failed due to deadline being exceeded for the attempt"
	}
	result, err := r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, reason, message, pLog)
	if err == nil {
		// Increment a counter metric for this cluster type and error reason:
		metricInstallErrors.Observe(instance, map[string]string{"reason": reason}, 1)
		metricClusterProvisionsTotal.Observe(instance, map[string]string{"result": resultFailure}, 1)
	}
	return result, err
}

func (r *ReconcileClusterProvision) startProvisioning(instance *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("provision initialization complete")
	return r.transitionStage(instance, hivev1.ClusterProvisionStageProvisioning, "InitializationComplete", "Install job has completed its initialization. Provisioning started.", pLog)
}

func (r *ReconcileClusterProvision) abortProvision(instance *hivev1.ClusterProvision, reason string, message string, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Infof("aborting provision (%s): %s", reason, message)
	if instance.Status.JobRef == nil {
		return r.transitionStage(instance, hivev1.ClusterProvisionStageFailed, reason, message, pLog)
	}
	if err := r.setCondition(instance, hivev1.ClusterProvisionFailedCondition, corev1.ConditionTrue, reason, message, controllerutils.UpdateConditionAlways, pLog); err != nil {
		return reconcile.Result{}, err
	}
	job := &batchv1.Job{}
	switch err := r.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Status.JobRef.Name}, job); {
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
	if err := r.setCondition(instance, conditionType, corev1.ConditionTrue, reason, message, controllerutils.UpdateConditionAlways, pLog); err != nil {
		return reconcile.Result{}, err
	}
	if stage == hivev1.ClusterProvisionStageFailed || stage == hivev1.ClusterProvisionStageComplete {
		r.logProvisionSuccessFailureMetric(stage, instance)
	}
	if err := r.setStage(instance, stage, pLog); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterProvision) setCondition(
	instance *hivev1.ClusterProvision,
	conditionType hivev1.ClusterProvisionConditionType,
	status corev1.ConditionStatus,
	reason string,
	message string,
	updateConditionCheck controllerutils.UpdateConditionCheck,
	pLog log.FieldLogger,
) error {
	instance.Status.Conditions = controllerutils.SetClusterProvisionCondition(
		instance.Status.Conditions,
		conditionType,
		status,
		reason,
		message,
		updateConditionCheck,
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

func clusterDeploymentWatchHandler(ctx context.Context, a client.Object) []reconcile.Request {
	cd := a.(*hivev1.ClusterDeployment)
	if cd == nil {
		// Wasn't a ClusterDeployment, bail out. This should not happen.
		log.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a)
		return nil
	}

	if cd.Status.ProvisionRef == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      cd.Status.ProvisionRef.Name,
				Namespace: cd.Namespace,
			},
		},
	}
}

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList: &batchv1.JobList{},
			LabelSelector: map[string]string{
				constants.ClusterProvisionNameLabel: owner.GetName(),
				constants.JobTypeLabel:              constants.JobTypeProvision,
			},
			Controlled: true,
		},
		{
			TypeToList: &corev1.SecretList{},
			LabelSelector: map[string]string{
				constants.ClusterProvisionNameLabel: owner.GetName(),
				constants.SecretTypeLabel:           constants.SecretTypeKubeConfig,
			},
			Controlled: false,
		},
		{
			TypeToList: &corev1.SecretList{},
			LabelSelector: map[string]string{
				constants.ClusterProvisionNameLabel: owner.GetName(),
				constants.SecretTypeLabel:           constants.SecretTypeKubeAdminCreds,
			},
			Controlled: false,
		},
	}
}

// deleteInstallJob deletes the install job of a successful provision
func (r *ReconcileClusterProvision) deleteInstallJob(provision *hivev1.ClusterProvision, pLog log.FieldLogger) (reconcile.Result, error) {
	pLog.Info("deleting successful install job")
	job := &batchv1.Job{}
	switch err := r.Get(context.TODO(), types.NamespacedName{Name: provision.Status.JobRef.Name, Namespace: provision.Namespace}, job); {
	case apierrors.IsNotFound(err):
		pLog.Info("install job has already been deleted")
	case err != nil:
		pLog.WithError(err).Log(controllerutils.LogLevel(err), "could not get install job")
		return reconcile.Result{}, err
	default:
		// deleting install job with background propagation policy to cascade delete install pod
		if err := r.Delete(context.TODO(), job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
			pLog.WithField("job", job.Name).WithError(err).Log(controllerutils.LogLevel(err), "error deleting successful install job")
			return reconcile.Result{}, err
		}
	}
	// clearing job reference after the install job has been deleted
	pLog.Info("clearing job reference after the install job has been deleted")
	provision.Status.JobRef = nil
	if err := r.Status().Update(context.TODO(), provision); err != nil {
		pLog.WithError(err).Log(controllerutils.LogLevel(err), "error clearing job reference after the install job has been deleted")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

// getWorkers fetches the number of worker replicas from the installConfig, and reports "unknown" if not available
func (r *ReconcileClusterProvision) getWorkers(cd hivev1.ClusterDeployment) string {
	icSecret := &corev1.Secret{}
	err := r.Get(context.Background(),
		types.NamespacedName{
			Namespace: cd.Namespace,
			Name:      cd.Spec.Provisioning.InstallConfigSecretRef.Name,
		},
		icSecret)
	if err != nil {
		r.logger.WithError(err).Error("Error loading install config secret")
		return "unknown"
	}
	ic := &installertypes.InstallConfig{}
	if err := yaml.Unmarshal(icSecret.Data["install-config.yaml"], &ic); err != nil {
		r.logger.WithError(err).Error("could not unmarshal InstallConfig")
		return "unknown"
	}
	mp := ic.WorkerMachinePool()
	if mp == nil || mp.Replicas == nil {
		return "0"
	}
	return strconv.FormatInt(*mp.Replicas, 10)
}

func (r *ReconcileClusterProvision) logProvisionSuccessFailureMetric(
	stage hivev1.ClusterProvisionStage, instance *hivev1.ClusterProvision) {
	cd := &hivev1.ClusterDeployment{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Namespace: instance.Namespace,
		Name: instance.Spec.ClusterDeploymentRef.Name}, cd); err != nil {
		r.logger.WithError(err).Error("error getting cluster deployment")
		return
	}
	timeMetric := metricInstallFailureSeconds
	if stage == hivev1.ClusterProvisionStageComplete {
		timeMetric = metricInstallSuccessSeconds
	}
	fixedLabels := map[string]string{
		"platform":        cd.Labels[hivev1.HiveClusterPlatformLabel],
		"region":          cd.Labels[hivev1.HiveClusterRegionLabel],
		"cluster_version": *cd.Status.InstallVersion,
		"workers":         r.getWorkers(*cd),
		"install_attempt": strconv.Itoa(instance.Spec.Attempt),
	}
	timeMetric.Observe(cd, fixedLabels, time.Since(instance.CreationTimestamp.Time).Seconds())
}

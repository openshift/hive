package clusterdeprovision

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
)

const (
	ControllerName                = hivev1.ClusterDeprovisionControllerName
	jobHashAnnotation             = "hive.openshift.io/jobhash"
	authenticationFailedReason    = "AuthenticationFailed"
	authenticationSucceededReason = "AuthenticationSucceeded"
)

var (
	metricUninstallJobDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_cluster_deployment_uninstall_job_duration_seconds",
			Help:    "Distribution of the runtime of completed uninstall jobs.",
			Buckets: []float64{60, 300, 600, 1200, 1800, 2400, 3000, 3600},
		},
	)

	// actuators is a list of available actuators for this controller
	// It is populated via the registerActuator function
	actuators []Actuator
)

// registerActuator registers an actuator with this controller. The actuator
// determines whether it can handle a particular cluster deployment via the CanHandle
// function.
func registerActuator(a Actuator) {
	actuators = append(actuators, a)
}

func init() {
	metrics.Registry.MustRegister(metricUninstallJobDuration)
}

// Add creates a new ClusterDeprovision Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	r, err := newReconciler(mgr, clientRateLimiter)
	if err != nil {
		return err
	}
	return add(mgr, r, concurrentReconciles, queueRateLimiter)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (reconcile.Reconciler, error) {
	deprovisionsDisabled := false
	if val, ok := os.LookupEnv(constants.DeprovisionsDisabledEnvVar); ok {
		var err error
		deprovisionsDisabled, err = strconv.ParseBool(val)
		if err != nil {
			log.WithError(err).WithField(constants.DeprovisionsDisabledEnvVar, os.Getenv(constants.DeprovisionsDisabledEnvVar)).
				Error("error parsing bool from env var")
			return nil, err
		}
	}
	return &ReconcileClusterDeprovision{
		Client:               controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:               mgr.GetScheme(),
		deprovisionsDisabled: deprovisionsDisabled,
	}, nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("clusterdeprovision-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error getting new clusterdeprovision-controller")
		return err
	}

	// Watch for changes to ClusterDeprovision
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeprovision{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching changes to clusterdeprovision")
		return err
	}

	// Watch for uninstall jobs created for ClusterDeprovisions
	err = c.Watch(source.Kind(mgr.GetCache(), &batchv1.Job{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeprovision{}, handler.OnlyControllerOwner()))
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching  uninstall jobs created for clusterdeprovisionreques")
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterDeprovision{}

// ReconcileClusterDeprovision reconciles a ClusterDeprovision object
type ReconcileClusterDeprovision struct {
	client.Client
	scheme               *runtime.Scheme
	deprovisionsDisabled bool
}

// Reconcile reads that state of the cluster for a ClusterDeprovision object and makes changes based on the state read
// and what is in the ClusterDeprovision.Spec
func (r *ReconcileClusterDeprovision) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeprovision", request.NamespacedName)
	// For logging, we need to see when the reconciliation loop starts and ends.
	rLog.Info("reconciling cluster deprovision request")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, rLog)
	defer recobsrv.ObserveControllerReconcileTime()

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
	rLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: instance}, rLog)

	// Ensure owner references are correctly set
	err = controllerutils.ReconcileOwnerReferences(instance, generateOwnershipUniqueKeys(instance), r, r.scheme, rLog)
	if err != nil {
		rLog.WithError(err).Error("Error reconciling object ownership")
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
	if controllerutils.IsDeleteProtected(cd) {
		rLog.Error("deprovision blocked for ClusterDeployment with protected delete on")
		return reconcile.Result{}, nil
	}

	// Check if deprovisions are currently disabled: (originates in HiveConfig in real world)
	if r.deprovisionsDisabled {
		rLog.Warn("deprovisions are currently disabled in HiveConfig, skipping")
		return reconcile.Result{}, nil
	}

	actuator := r.getActuator(instance)
	if actuator == nil {
		rLog.Debug("No actuator found for this provider")
	} else {
		// actuator found, ensure creds work.
		err := actuator.TestCredentials(instance, r.Client, rLog)
		if err != nil {
			rLog.WithError(err).Warn("Credential check failed")

			conditions, changed := controllerutils.SetClusterDeprovisionConditionWithChangeCheck(
				instance.Status.Conditions,
				hivev1.AuthenticationFailureClusterDeprovisionCondition,
				corev1.ConditionTrue,
				authenticationFailedReason,
				"Credential check failed",
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			)

			if changed {
				instance.Status.Conditions = conditions
				if updateErr := r.Status().Update(context.Background(), instance); updateErr != nil {
					return reconcile.Result{}, updateErr
				}
			}

			return reconcile.Result{}, err
		}

		// Authentication succeeded. Make sure that's noted in status.
		conditions, changed := controllerutils.SetClusterDeprovisionConditionWithChangeCheck(
			instance.Status.Conditions,
			hivev1.AuthenticationFailureClusterDeprovisionCondition,
			corev1.ConditionFalse,
			authenticationSucceededReason,
			"Credential check succeeded",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)

		if changed {
			instance.Status.Conditions = conditions
			if err := r.Status().Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	extraEnvVars := getAWSServiceProviderEnvVars(instance, instance.Name)

	if err := install.CopyAWSServiceProviderSecret(r.Client, instance.Namespace, extraEnvVars, instance, r.scheme); err != nil {
		rLog.WithError(err).Error("could not copy AWS service provider secret")
		return reconcile.Result{}, err
	}

	if err := r.setupAWSCredentialForAssumeRole(instance); err != nil {
		if !errors.IsAlreadyExists(err) {
			// Couldn't create the assume role credentials secret for a reason other than it already exists.
			// If the secret already exists, then we should just use that secret.
			rLog.WithError(err).Error("could not create assume role AWS secret")
			return reconcile.Result{}, err
		}
	}

	if err := controllerutils.SetupClusterUninstallServiceAccount(r, cd.Namespace, rLog); err != nil {
		rLog.WithError(err).Log(controllerutils.LogLevel(err), "error setting up service account and role")
		return reconcile.Result{}, err
	}

	// Generate an uninstall job
	rLog.Debug("generating uninstall job")
	uninstallJob, err := install.GenerateUninstallerJobForDeprovision(instance,
		controllerutils.UninstallServiceAccountName,
		os.Getenv("HTTP_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("NO_PROXY"),
		extraEnvVars)
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
		conditions, _ := controllerutils.SetClusterDeprovisionConditionWithChangeCheck(
			instance.Status.Conditions,
			hivev1.DeprovisionFailedClusterDeprovisionCondition,
			corev1.ConditionFalse,
			"DeprovisionCompleted",
			"Deprovision has succeeded",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		instance.Status.Conditions = conditions

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
	if controllerutils.IsFailed(existingJob) {
		newJobNeeded = true
		reason, message := "UnknownError", "Deprovision attempt failed for unknown reason"
		if controllerutils.IsDeadlineExceeded(existingJob) {
			reason, message = "AttemptDeadlineExceeded", "Deprovision attempt failed because the deadline was exceeded"
		}
		conditions, changed := controllerutils.SetClusterDeprovisionConditionWithChangeCheck(
			instance.Status.Conditions,
			hivev1.DeprovisionFailedClusterDeprovisionCondition,
			corev1.ConditionTrue,
			reason, message,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			instance.Status.Conditions = conditions
			if err := r.Status().Update(context.Background(), instance); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	if newJobNeeded {
		if existingJob.DeletionTimestamp == nil {
			rLog.Info("deleting existing deprovision job due to updated/missing hash detected")
			err := r.Delete(context.TODO(), existingJob, client.PropagationPolicy(metav1.DeletePropagationForeground))
			if err != nil {
				rLog.WithError(err).Log(controllerutils.LogLevel(err), "error deleting outdated deprovision job")
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	rLog.Infof("uninstall job not yet successful")
	return reconcile.Result{}, nil
}

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList: &batchv1.JobList{},
			LabelSelector: map[string]string{
				constants.ClusterDeprovisionNameLabel: owner.GetName(),
				constants.JobTypeLabel:                constants.JobTypeDeprovision,
			},
			Controlled: true,
		},
	}
}

func (r *ReconcileClusterDeprovision) getActuator(cd *hivev1.ClusterDeprovision) Actuator {
	for _, a := range actuators {
		if a.CanHandle(cd) {
			return a
		}
	}
	return nil
}

func getAWSServiceProviderEnvVars(cd *hivev1.ClusterDeprovision, secretPrefix string) []corev1.EnvVar {
	var extraEnvVars []corev1.EnvVar
	spSecretName := os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar)
	if spSecretName == "" {
		return extraEnvVars
	}

	if cd.Spec.Platform.AWS == nil {
		return extraEnvVars
	}

	extraEnvVars = append(extraEnvVars, corev1.EnvVar{
		Name:  constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar,
		Value: secretPrefix + "-" + spSecretName,
	})
	return extraEnvVars
}

func (r *ReconcileClusterDeprovision) setupAWSCredentialForAssumeRole(cd *hivev1.ClusterDeprovision) error {
	if cd.Spec.Platform.AWS == nil ||
		cd.Spec.Platform.AWS.CredentialsSecretRef.Name != "" ||
		cd.Spec.Platform.AWS.CredentialsAssumeRole == nil {
		// no setup required
		return nil
	}

	return install.AWSAssumeRoleCLIConfig(r.Client, cd.Spec.Platform.AWS.CredentialsAssumeRole, install.AWSAssumeRoleSecretName(cd.Name), cd.Namespace, cd, r.scheme)

}

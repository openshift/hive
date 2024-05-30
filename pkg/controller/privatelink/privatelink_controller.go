package privatelink

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/retry"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	"github.com/openshift/hive/pkg/controller/privatelink/dnsrecord"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = hivev1.PrivateLinkControllerName
	finalizer      = "hive.openshift.io/private-link"

	lastCleanupAnnotationKey = "private-link-controller.hive.openshift.io/last-cleanup-for"
)

var _ reconcile.Reconciler = &ReconcilePrivateLink{}

// ReconcilePrivateLink reconciles a PrivateLink for clusterdeployment object
type ReconcilePrivateLink struct {
	client.Client

	hubActuator actuator.Actuator

	controllerconfig *hivev1.PrivateLinkConfig
}

// Add creates a new PrivateLink Controller and adds it to the Manager with default RBAC.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	reconciler, err := NewReconciler(mgr, clientRateLimiter)
	if err != nil {
		logger.WithError(err).Error("could not create reconciler")
		return err
	}
	return AddToManager(mgr, reconciler, concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new ReconcilePrivateLink
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) (*ReconcilePrivateLink, error) {
	logger := log.WithField("controller", ControllerName)
	reconciler := &ReconcilePrivateLink{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
	}

	var err error
	if reconciler.controllerconfig, err = ReadPrivateLinkControllerConfigFile(); err != nil {
		logger.WithError(err).Error("could not load configuration")
		return reconciler, err
	}

	// TODO: Determine and use hub platform type
	// Ideally we would get the hub platform type from the cluster infrastructure. However,
	// the hive service account does not have access. For now, we hard-code aws platform.
	var platformType = configv1.AWSPlatformType

	if reconciler.hubActuator, err = reconciler.GetActuator(platformType, actuator.ActuatorTypeHub, logger); err != nil {
		logger.WithError(err).Error("could not get hub actuator")
		return reconciler, err
	}
	logger.Debugf("hub actuator created: %s", platformType)

	return reconciler, nil
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r *ReconcilePrivateLink, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	logger := log.WithField("controller", ControllerName)
	// Create a new controller
	c, err := controller.New(string(ControllerName+"-controller"), mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		logger.WithError(err).Error("error creating new controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		logger.WithError(err).Error("error watching cluster deployment")
		return err
	}

	// Watch for changes to ClusterProvision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterProvision{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner())); err != nil {
		logger.WithError(err).Error("error watching cluster provision")
		return err
	}

	// Watch for changes to ClusterDeprovision
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeprovision{}),
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.ClusterDeployment{}, handler.OnlyControllerOwner())); err != nil {
		logger.WithError(err).Error("error watching cluster deprovision")
		return err
	}

	return nil
}

func (r *ReconcilePrivateLink) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	logger.Debug("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	if r.hubActuator == nil {
		logger.Error("hub actuator is not set")
		return reconcile.Result{}, errors.New("hub actuator is not set")
	}

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if apierrors.IsNotFound(err) {
		logger.Debug("cluster deployment not found")
		return reconcile.Result{}, nil
	}
	if err != nil {
		// Error reading the object - requeue the request.
		logger.WithError(err).Error("error getting ClusterDeployment")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, logger)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		logger.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := conditions.InitializeConditions(cd)
	if changed {
		cd.Status.Conditions = newConditions
		logger.Info("initializing private link controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	var privateLinkEnabled bool
	var spokePlatformType configv1.PlatformType
	switch {
	case cd.Spec.Platform.AWS != nil:
		spokePlatformType = configv1.AWSPlatformType
		if cd.Spec.Platform.AWS.PrivateLink != nil {
			privateLinkEnabled = cd.Spec.Platform.AWS.PrivateLink.Enabled
		}
	case cd.Spec.Platform.GCP != nil:
		spokePlatformType = configv1.GCPPlatformType
	//	if cd.Spec.Platform.GCP.PrivateLink != nil {
	//		privateLinkEnabled = cd.Spec.Platform.GCP.PrivateLink.Enabled
	//	}
	default:
		logger.Debug("controller cannot service the clusterdeployment, so skipping")
		return reconcile.Result{}, nil
	}

	linkActuator, err := r.GetActuator(spokePlatformType, actuator.ActuatorTypeLink, logger)
	if err != nil {
		logger.WithError(err).Error("could not get link actuator")
		return reconcile.Result{}, err
	}

	if !privateLinkEnabled {
		if r.cleanupRequired(cd, linkActuator) {
			// private link was disabled for this cluster so cleanup is required.
			return r.cleanupClusterDeployment(cd, linkActuator, logger)
		}

		logger.Debug("cluster deployment does not have private link enabled, so skipping")
		return reconcile.Result{}, nil
	}

	if cd.DeletionTimestamp != nil {
		return r.cleanupClusterDeployment(cd, linkActuator, logger)
	}

	if err = r.hubActuator.Validate(cd, logger); err != nil {
		return reconcile.Result{}, err
	}

	if err = linkActuator.Validate(cd, logger); err != nil {
		return reconcile.Result{}, err
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(cd, finalizer) {
		logger.Debug("adding finalizer to ClusterDeployment")
		controllerutils.AddFinalizer(cd, finalizer)
		if err := r.Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "error adding finalizer to ClusterDeployment")
			return reconcile.Result{}, err
		}
	}

	// See if we need to sync. This is what rate limits our cloud API usage, but allows for immediate syncing
	// on changes and deletes.
	shouldSync, syncAfter := r.shouldSync(cd, linkActuator)
	if !shouldSync {
		logger.WithFields(log.Fields{
			"syncAfter": syncAfter,
		}).Debug("Sync not needed")

		return reconcile.Result{RequeueAfter: syncAfter}, nil
	}

	if cd.Spec.Installed {
		logger.Debug("reconciling already installed cluster deployment")
		return r.reconcilePrivateLink(cd, cd.Spec.ClusterMetadata, linkActuator, logger)
	}

	if cd.Status.ProvisionRef == nil {
		logger.Debug("waiting for cluster deployment provision to start, will retry soon.")
		return reconcile.Result{}, nil
	}

	cpLog := logger.WithField("provision", cd.Status.ProvisionRef.Name)
	cp := &hivev1.ClusterProvision{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.ProvisionRef.Name, Namespace: cd.Namespace}, cp)
	if apierrors.IsNotFound(err) {
		cpLog.Warn("linked cluster provision not found")
		return reconcile.Result{}, err
	}
	if err != nil {
		cpLog.WithError(err).Error("could not get provision")
		return reconcile.Result{}, err
	}

	if cp.Spec.PrevInfraID != nil && *cp.Spec.PrevInfraID != "" && r.cleanupRequired(cd, linkActuator) {
		lastCleanup := cd.Annotations[lastCleanupAnnotationKey]
		if lastCleanup != *cp.Spec.PrevInfraID {
			logger.WithField("prevInfraID", *cp.Spec.PrevInfraID).
				Info("cleaning up PrivateLink resources from previous attempt")

			if err := r.cleanupPreviousProvisionAttempt(cd, cp, linkActuator, logger); err != nil {
				logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")

				if err := conditions.SetErrCondition(cd, "CleanupForProvisionReattemptFailed", err, r.Client, logger); err != nil {
					logger.WithError(err).Error("failed to update condition on cluster deployment")
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, err
			}

			if err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
				"PreviousAttemptCleanupComplete",
				"successfully cleaned up resources from previous provision attempt so that next attempt can start",
				r.Client, logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
				return reconcile.Result{}, err
			}

			return reconcile.Result{Requeue: true}, nil
		}
	}

	if cp.Spec.InfraID == nil ||
		(cp.Spec.InfraID != nil && *cp.Spec.InfraID == "") ||
		(cp.Spec.AdminKubeconfigSecretRef == nil) ||
		(cp.Spec.AdminKubeconfigSecretRef != nil && cp.Spec.AdminKubeconfigSecretRef.Name == "") {
		logger.Debug("waiting for cluster deployment provision to provide ClusterMetadata, will retry soon.")
		return reconcile.Result{}, nil
	}

	return r.reconcilePrivateLink(cd, &hivev1.ClusterMetadata{InfraID: *cp.Spec.InfraID, AdminKubeconfigSecretRef: *cp.Spec.AdminKubeconfigSecretRef}, linkActuator, logger)
}

func (r *ReconcilePrivateLink) GetActuator(platform configv1.PlatformType, actuatorType actuator.ActuatorType, logger log.FieldLogger) (actuator.Actuator, error) {
	switch platform {
	case configv1.AWSPlatformType:
		var awsConfig *hivev1.AWSPrivateLinkConfig
		if r.controllerconfig != nil {
			awsConfig = r.controllerconfig.AWS
		}
		if actuatorType == actuator.ActuatorTypeHub {
			return awsactuator.NewAWSHubActuator(&r.Client, awsConfig, logger)
		}
		if actuatorType == actuator.ActuatorTypeLink {
			return awsactuator.NewAWSLinkActuator(&r.Client, awsConfig, logger)
		}
		return nil, fmt.Errorf("unable to create privatelink actuator, invalid actuator type: %s/%s", platform, actuatorType)
	}
	return nil, fmt.Errorf("unable to create privatelink actuator, invalid platform: %s", platform)
}

func (r *ReconcilePrivateLink) cleanupRequired(cd *hivev1.ClusterDeployment, linkActuator actuator.Actuator) bool {
	return r.hubActuator.CleanupRequired(cd) ||
		linkActuator.CleanupRequired(cd)
}

func (r *ReconcilePrivateLink) cleanupClusterDeployment(cd *hivev1.ClusterDeployment, linkActuator actuator.Actuator, logger log.FieldLogger) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(cd, finalizer) {
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata != nil && r.cleanupRequired(cd, linkActuator) {
		if err := r.cleanupPrivateLink(cd, cd.Spec.ClusterMetadata, linkActuator, logger); err != nil {
			logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")

			if err := conditions.SetErrCondition(cd, "CleanupForDeprovisionFailed", err, r.Client, logger); err != nil {
				logger.WithError(err).Error("failed to update condition on cluster deployment")
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, err
		}

		if err := conditions.SetReadyCondition(cd, corev1.ConditionFalse,
			"DeprovisionCleanupComplete",
			"successfully cleaned up private link resources created to deprovision cluster",
			r.Client, logger); err != nil {
			logger.WithError(err).Error("failed to update condition on cluster deployment")
			return reconcile.Result{}, err
		}
	}

	logger.Info("removing finalizer from ClusterDeployment")
	controllerutils.DeleteFinalizer(cd, finalizer)
	if err := r.Update(context.Background(), cd); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer from ClusterDeployment")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePrivateLink) cleanupPrivateLink(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	linkActuator actuator.Actuator, logger log.FieldLogger) error {
	err := r.hubActuator.Cleanup(cd, metadata, logger)
	if err != nil {
		return err
	}
	return linkActuator.Cleanup(cd, metadata, logger)
}

func (r *ReconcilePrivateLink) reconcilePrivateLink(cd *hivev1.ClusterDeployment, metadata *hivev1.ClusterMetadata,
	linkActuator actuator.Actuator, logger log.FieldLogger) (reconcile.Result, error) {

	dnsRecord := &dnsrecord.DnsRecord{}
	reconcileResult, err := linkActuator.Reconcile(cd, metadata, dnsRecord, logger)
	if err != nil {
		return reconcileResult, err
	}

	if !reconcileResult.IsZero() {
		return reconcileResult, nil
	}

	return r.hubActuator.Reconcile(cd, metadata, dnsRecord, logger)
}

func (r *ReconcilePrivateLink) cleanupPreviousProvisionAttempt(cd *hivev1.ClusterDeployment, cp *hivev1.ClusterProvision,
	linkActuator actuator.Actuator, logger log.FieldLogger) error {
	if cd.Spec.ClusterMetadata == nil {
		return errors.New("cannot cleanup previous resources because the admin kubeconfig is not available")
	}
	metadata := &hivev1.ClusterMetadata{
		InfraID:                  *cp.Spec.PrevInfraID,
		AdminKubeconfigSecretRef: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef,
	}

	if err := r.cleanupPrivateLink(cd, metadata, linkActuator, logger); err != nil {
		logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")
		return err
	}
	if cd.Annotations == nil {
		cd.Annotations = map[string]string{}
	}
	cd.Annotations[lastCleanupAnnotationKey] = metadata.InfraID
	return updateAnnotations(r.Client, cd)
}

// shouldSync returns if we should sync the desired ClusterDeployment. If it returns false, it also returns
// the duration after which we should try to check if sync is required.
func (r *ReconcilePrivateLink) shouldSync(desired *hivev1.ClusterDeployment, linkActuator actuator.Actuator) (bool, time.Duration) {
	window := 2 * time.Hour
	if desired.DeletionTimestamp != nil && !controllerutils.HasFinalizer(desired, finalizer) {
		return false, 0 // No finalizer means our cleanup has been completed. There's nothing left to do.
	}

	if desired.DeletionTimestamp != nil {
		return true, 0 // We're in a deleting state, sync now.
	}

	failedCondition := controllerutils.FindCondition(desired.Status.Conditions, hivev1.PrivateLinkFailedClusterDeploymentCondition)
	if failedCondition != nil && failedCondition.Status == corev1.ConditionTrue {
		return true, 0 // we have failed to reconcile and therefore should continue to retry for quick recovery
	}

	readyCondition := controllerutils.FindCondition(desired.Status.Conditions, hivev1.PrivateLinkReadyClusterDeploymentCondition)
	if readyCondition == nil || readyCondition.Status != corev1.ConditionTrue {
		return true, 0 // we have not reached Ready level
	}
	delta := time.Since(readyCondition.LastProbeTime.Time)

	if !desired.Spec.Installed {
		// as cluster is installing, but the private link has been setup once, we wait
		// for a shorter duration before reconciling again.
		window = 10 * time.Minute
	}

	if delta >= window {
		// We haven't sync'd in over resync duration time, sync now.
		return true, 0
	}

	syncAfter := (window - delta).Round(time.Minute)
	if syncAfter == 0 {
		// if it is less than a minute, sync after a minute
		syncAfter = time.Minute
	}

	if r.hubActuator.ShouldSync(desired) ||
		linkActuator.ShouldSync(desired) {
		// if there are desired changes to be made
		return true, 0
	}

	// We didn't meet any of the criteria above, so we should not sync.
	return false, syncAfter
}

// ReadPrivateLinkControllerConfigFile reads the configuration from the env
// and unmarshals. If the env is set to a file but that file doesn't exist it returns
// a zero-value configuration.
func ReadPrivateLinkControllerConfigFile() (*hivev1.PrivateLinkConfig, error) {
	fPath := os.Getenv(constants.PrivateLinkControllerConfigFileEnvVar)
	if len(fPath) == 0 {
		return nil, nil
	}

	config := &hivev1.PrivateLinkConfig{}

	fileBytes, err := os.ReadFile(fPath)
	if os.IsNotExist(err) {
		return config, nil
	}
	if err != nil {
		return config, errors.Wrap(err, "failed to read the privatelink controller config file")
	}
	if err := json.Unmarshal(fileBytes, &config); err != nil {
		return config, err
	}

	return config, nil
}

func updateAnnotations(client client.Client, cd *hivev1.ClusterDeployment) error {
	var retryBackoff = wait.Backoff{
		Steps:    5,
		Duration: 1 * time.Second,
		Factor:   1.0,
		Jitter:   0.1,
	}
	return retry.RetryOnConflict(retryBackoff, func() error {
		curr := &hivev1.ClusterDeployment{}
		err := client.Get(context.TODO(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, curr)
		if err != nil {
			return err
		}
		curr.Annotations = cd.Annotations
		return client.Update(context.TODO(), curr)
	})
}

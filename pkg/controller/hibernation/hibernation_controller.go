package hibernation

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	machineapi "github.com/openshift/api/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	// ControllerName is the name of this controller
	ControllerName = hivev1.HibernationControllerName

	// stateCheckInterval is the time interval for polling
	// whether a cluster's machines are stopped or are running
	stateCheckInterval = 1 * time.Minute

	// csrCheckInterval is the time interval for polling
	// pending CertificateSigningRequests
	csrCheckInterval = 30 * time.Second

	// clusterOperatorCheckInterval is the time interval for polling
	// ClusterOperator state
	clusterOperatorCheckInterval = 30 * time.Second

	// nodeCheckWaitTime is the minimum time to wait for a node
	// ready check after a cluster started resuming. This is to
	// avoid a false positive when the node status is checked too
	// soon after the cluster is ready
	nodeCheckWaitTime = 4 * time.Minute

	// hibernateAfterSyncSetsNotApplied is the amount of time to wait
	// before hibernating when SyncSets have not been applied
	hibernateAfterSyncSetsNotApplied = 10 * time.Minute

	machineAPINamespace                 = "openshift-machine-api"
	machineAPIInterruptibleLabel        = "machine.openshift.io/interruptible-instance"
	machineAPIExcludeDrainingAnnotation = "machine.openshift.io/exclude-node-draining"

	clusterRunningMsg           = "Cluster is running"
	clusterResumingOrRunningMsg = "Cluster is resuming or running, see Ready condition for details"
	clusterHibernatingMsg       = "Cluster is shutting down or hibernating, see Hibernating condition for details"
)

var (
	// actuators is a list of available actuators for this controller
	// It is populated via the RegisterActuator function
	actuators []HibernationActuator

	// clusterDeploymentHibernationConditions are the cluster deployment conditions controlled by
	// hibernation controller
	clusterDeploymentHibernationConditions = []hivev1.ClusterDeploymentConditionType{
		hivev1.ClusterHibernatingCondition,
		hivev1.ClusterReadyCondition,
	}
)

// Add creates a new Hibernation controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// RegisterActuator register an actuator with this controller. The actuator
// determines whether it can handle a particular cluster deployment via the CanHandle
// function.
func RegisterActuator(a HibernationActuator) {
	actuators = append(actuators, a)
}

// hibernationReconciler is the reconciler type for this controller
type hibernationReconciler struct {
	client.Client
	logger  log.FieldLogger
	csrUtil csrHelper

	remoteClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// NewReconciler returns a new Reconciler
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) *hibernationReconciler {
	logger := log.WithField("controller", ControllerName)
	r := &hibernationReconciler{
		Client:  controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		logger:  logger,
		csrUtil: &csrUtility{},
	}
	r.remoteClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// AddToManager adds a new Controller to the controller manager
func AddToManager(mgr manager.Manager, r *hibernationReconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	c, err := controller.New("hibernation-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Log(controllerutils.LogLevel(err), "Error creating controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}},
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Log(controllerutils.LogLevel(err), "Error setting up a watch on ClusterDeployment")
		return err
	}
	return nil
}

// Reconcile syncs a single ClusterDeployment
func (r *hibernationReconciler) Reconcile(ctx context.Context, request reconcile.Request) (result reconcile.Result, returnErr error) {
	cdLog := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			cdLog.Info("cluster deployment Not Found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "Error getting cluster deployment")
		return reconcile.Result{}, err
	}
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// If cluster is already deleted, skip any processing
	if !cd.DeletionTimestamp.IsZero() {
		cd.Status.PowerState = hivev1.ClusterPowerStateUnknown
		msg := "ClusterDeployment has been marked for deletion"
		r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonClusterDeploymentDeleted, msg, corev1.ConditionUnknown, cdLog)
		r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonClusterDeploymentDeleted, msg, corev1.ConditionUnknown, cdLog)
		return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentHibernationConditions)
	if changed {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing hibernating controller conditions")
		if err := r.updateClusterDeploymentStatus(cd, cdLog); err != nil {
			return reconcile.Result{}, err
		}
	}

	// If cluster is not installed, skip any processing
	if !cd.Spec.Installed {
		return reconcile.Result{}, nil
	}

	// Secret toggle allowing consumers to e.g. perform manual maintenance on machines without the
	// controller attempting to reconcile desired CD.Spec.PowerState.
	if paused, err := strconv.ParseBool(cd.Annotations[constants.PowerStatePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile of PowerState as the powerstate-pause annotation is set")
		changed := false
		if cd.Status.PowerState != hivev1.ClusterPowerStateUnknown {
			changed = true
			cd.Status.PowerState = hivev1.ClusterPowerStateUnknown
		}
		msg := "the powerstate-pause annotation is set"
		changed = r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonPowerStatePaused, msg, corev1.ConditionUnknown, cdLog) ||
			changed
		changed = r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonPowerStatePaused, msg, corev1.ConditionUnknown, cdLog) ||
			changed
		if changed {
			return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
		}
		return reconcile.Result{}, nil
	}

	hibernatingCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)
	readyCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterReadyCondition)

	if supported, msg := r.hibernationSupported(cd); !supported {
		// Set hibernating condition to false for unsupported clouds.
		// Set Ready condition and Status.PowerState to indicate "Running" for consistent UX.
		changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonUnsupported, msg,
			corev1.ConditionFalse, cdLog)
		changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonRunning,
			"No power state actuator -- assuming running", corev1.ConditionTrue, cdLog)
		changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateRunning
		if changed || changed2 || changed3 {
			cd.Status.PowerState = hivev1.ClusterPowerStateRunning
			return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
		}
	} else if hibernatingCondition.Reason == hivev1.HibernatingReasonUnsupported {
		// Clear any lingering unsupported hibernation condition
		r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning, msg,
			corev1.ConditionFalse, cdLog)
		return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
	}

	isFakeCluster := controllerutils.IsFakeCluster(cd)

	clusterSync := &hiveintv1alpha1.ClusterSync{}
	if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, clusterSync); err != nil {
		// This may be NotFound, which means the clustersync controller hasn't created the ClusterSync yet.
		// Fail and requeue to wait for it to exist.
		return reconcile.Result{}, fmt.Errorf("could not get ClusterSync: %v", err)
	}
	syncSetsApplied := clusterSync.Status.FirstSuccessTime != nil

	// Check on the SyncSetsNotApplied condition. Usually this is happening on a freshly installed cluster that's
	// running; checkClusterRunning will discover that state and flip the condition appropriately.
	if hibernatingCondition.Reason == hivev1.HibernatingReasonSyncSetsNotApplied && syncSetsApplied {
		r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonSyncSetsApplied,
			"SyncSets have been applied", corev1.ConditionFalse, cdLog)
		return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
	}

	shouldHibernate := cd.Spec.PowerState == hivev1.ClusterPowerStateHibernating
	// set readyToHibernate if hibernate after is ready to kick in hibernation
	var readyToHibernate bool

	// Check if HibernateAfter is set, and decide to hibernate or requeue
	if cd.Spec.HibernateAfter != nil && !shouldHibernate {
		// As the baseline timestamp for determining whether HibernateAfter should trigger, we use the latest of:
		// - When the cluster finished installing (status.installedTimestamp)
		// - When the cluster was claimed (spec.clusterPoolRef.claimedTimestamp -- ClusterPool CDs only)
		// - The last time the cluster resumed (status.conditions[Hibernating].lastTransitionTime if not hibernating (but see TODO))
		// BUT pool clusters wait until they're claimed for HibernateAfter to have effect.
		poolRef := cd.Spec.ClusterPoolRef
		isUnclaimedPoolCluster := poolRef != nil && poolRef.PoolName != "" &&
			// Upgrade note: If we hit this code path on a CD that was claimed before upgrading to
			// where we introduced ClaimedTimestamp, then that CD was Hibernating when it was claimed
			// (because that's the same time we introduced ClusterPool.RunningCount) so it's safe to
			// just use installed/last-resumed as the baseline for hibernateAfter.
			(poolRef.ClaimName == "" || poolRef.ClaimedTimestamp == nil)
		if !isUnclaimedPoolCluster {
			hibernateAfterDur := cd.Spec.HibernateAfter.Duration
			hibLog := cdLog.WithField("hibernateAfter", hibernateAfterDur)

			// The nil values of each of these will make them "earliest" and therefore unused.
			var installedSince, runningSince, claimedSince time.Time
			var isRunning bool

			installedSince = cd.Status.InstalledTimestamp.Time
			hibLog = hibLog.WithField("installedSince", installedSince)

			if poolRef != nil && poolRef.ClaimedTimestamp != nil {
				claimedSince = poolRef.ClaimedTimestamp.Time
				hibLog = hibLog.WithField("claimedSince", claimedSince)
			} else {
				// This means it's not a pool cluster (!isUnclaimedPoolCluster && poolRef == nil)
				hibLog.Debug("cluster does not belong to a clusterpool")
			}

			if hibernatingCondition.Status == corev1.ConditionUnknown {
				hibLog.Debug("cluster has never been hibernated")
				isRunning = true
			}
			if readyCondition.Status == corev1.ConditionTrue {
				runningSince = readyCondition.LastTransitionTime.Time
				hibLog = hibLog.WithField("runningSince", runningSince)
				isRunning = true
			}

			if isRunning {
				// Which timestamp should we use to calculate HibernateAfter from?
				// Sort our timestamps in descending order and use the first (latest) one.
				stamps := []time.Time{installedSince, claimedSince, runningSince}
				sort.Slice(stamps, func(i, j int) bool {
					return stamps[i].After(stamps[j])
				})
				expiry := stamps[0].Add(hibernateAfterDur)
				hibLog.Debugf("cluster should be hibernating after: %s", expiry)
				if time.Now().After(expiry) {
					hibLog.WithField("expiry", expiry).Debug("cluster has been running longer than hibernate-after duration, moving to hibernating powerState")
					readyToHibernate = true
				} else {
					requeueNow := result.Requeue && result.RequeueAfter <= 0
					if returnErr == nil && !requeueNow {
						// We have the hibernateAfter time but cluster has not been running that long yet.
						// Set requeueAfter for just after so that we requeue cluster for hibernation once reconcile has completed
						requeueAfter := time.Until(expiry)
						if requeueAfter < result.RequeueAfter || result.RequeueAfter <= 0 {
							hibLog.Infof("cluster will reconcile due to hibernate-after time in: %v", requeueAfter)
							result.RequeueAfter = requeueAfter
							result.Requeue = true
							return result, err
						}
					}
				}
			}
		}
	}

	if shouldHibernate || readyToHibernate {
		// Signal a problem if we should be hibernating and the SyncSets have not yet been applied.
		if !syncSetsApplied {
			// Allow hibernation (do not set condition) if hibernateAfterSyncSetsNotApplied duration has passed since cluster
			// installed and syncsets still not applied
			if cd.Status.InstalledTimestamp != nil && time.Since(cd.Status.InstalledTimestamp.Time) < hibernateAfterSyncSetsNotApplied {
				changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonSyncSetsNotApplied,
					"Cluster SyncSets have not been applied", corev1.ConditionFalse, cdLog)
				if changed {
					cd.Status.PowerState = hivev1.ClusterPowerStateSyncSetsNotApplied
					if err := r.updateClusterDeploymentStatus(cd, cdLog); err != nil {
						return reconcile.Result{}, err
					}
				}
				expiry := cd.Status.InstalledTimestamp.Time.Add(hibernateAfterSyncSetsNotApplied)
				requeueAfter := time.Until(expiry)
				cdLog.WithField("requeueAfter", requeueAfter).Infof("syncsets not yet applied, cluster will rereconcile")
				return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
			}
		}
		// Hibernate fake cluster
		if isFakeCluster && cd.Spec.HibernateAfter == nil && hibernatingCondition.Status != corev1.ConditionTrue {
			r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonHibernating,
				"Fake cluster is stopped", corev1.ConditionTrue, cdLog)
			r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStoppingOrHibernating,
				clusterHibernatingMsg, corev1.ConditionFalse, cdLog)
			cd.Status.PowerState = hivev1.ClusterPowerStateHibernating
			return reconcile.Result{}, r.updateClusterDeploymentStatus(cd, cdLog)
		}
		if readyToHibernate {
			cd.Spec.PowerState = hivev1.ClusterPowerStateHibernating
			err := r.Update(context.TODO(), cd)
			if err != nil {
				cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error hibernating cluster")
			}
			return reconcile.Result{}, err
		}
		// Stop machines if necessary; or poll whether they have stopped
		if shouldStopMachines(cd, hibernatingCondition) {
			return r.stopMachines(cd, cdLog)
		}
		return r.checkClusterStopped(cd, false, cdLog)
	}
	// If we get here, we're not supposed to be hibernating
	if isFakeCluster {
		changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning,
			clusterResumingOrRunningMsg, corev1.ConditionFalse, cdLog)
		rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonRunning, clusterRunningMsg,
			corev1.ConditionTrue, cdLog)
		if changed || rChanged {
			cd.Status.PowerState = hivev1.ClusterPowerStateRunning
			if err := r.updateClusterDeploymentStatus(cd, cdLog); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}
	// Start machines if necessary; or poll whether they have started
	if shouldStartMachines(cd, hibernatingCondition, readyCondition) {
		return r.startMachines(cd, cdLog)
	}
	return r.checkClusterRunning(cd, syncSetsApplied, cdLog, readyCondition)
}

func (r *hibernationReconciler) startMachines(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to start cluster machines")
		return reconcile.Result{}, nil
	}
	logger.Info("Resuming cluster")
	changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning,
		clusterResumingOrRunningMsg, corev1.ConditionFalse, logger)
	rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStartingMachines,
		"Starting cluster machines (step 1/4)", corev1.ConditionFalse, logger)
	if rChanged {
		cd.Status.PowerState = hivev1.ClusterPowerStateStartingMachines
	}
	err := actuator.StartMachines(cd, r.Client, logger)
	if err != nil {
		msg := fmt.Sprintf("Failed to start machines: %v", err)
		r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonFailedToStartMachines, msg,
			corev1.ConditionFalse, logger)
		cd.Status.PowerState = hivev1.ClusterPowerStateFailedToStartMachines
	}
	if changed || rChanged {
		if updateErr := r.updateClusterDeploymentStatus(cd, logger); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
	}
	// Return the error (if occurred) starting machines, so we get requeue + backoff
	return reconcile.Result{}, err
}

func (r *hibernationReconciler) stopMachines(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to start cluster machines")
		return reconcile.Result{}, nil
	}
	logger.Info("Stopping cluster")
	changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonStopping,
		"Stopping cluster machines", corev1.ConditionFalse, logger)
	rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStoppingOrHibernating,
		clusterHibernatingMsg, corev1.ConditionFalse, logger)
	if changed {
		cd.Status.PowerState = hivev1.ClusterPowerStateStopping
	}
	err := actuator.StopMachines(cd, r.Client, logger)
	if err != nil {
		msg := fmt.Sprintf("Failed to stop machines: %v", err)
		changed = r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonFailedToStop, msg,
			corev1.ConditionFalse, logger)
		cd.Status.PowerState = hivev1.ClusterPowerStateFailedToStop
	}
	if changed || rChanged {
		if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
			return reconcile.Result{}, err
		}
	}
	// Return the error (if occurred) stopping machines, so we get requeue + backoff
	return reconcile.Result{}, err
}

func (r *hibernationReconciler) checkClusterStopped(cd *hivev1.ClusterDeployment, expectRunning bool, logger log.FieldLogger) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to check machine status")
		return reconcile.Result{}, nil
	}

	stopped, remaining, err := actuator.MachinesStopped(cd, r.Client, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether machines are stopped.")
		return reconcile.Result{}, err
	}
	if !stopped {
		// Ensure all machines have been stopped. Should have been handled already but we've seen VMs left in running state.
		if err := actuator.StopMachines(cd, r.Client, logger); err != nil {
			logger.WithError(err).Error("error stopping machines")
			return reconcile.Result{}, err
		}

		sort.Strings(remaining) // we want to make sure the message is stable.
		msg := fmt.Sprintf("Stopping cluster machines. Some machines have not yet stopped: %s", strings.Join(remaining, ","))
		changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonWaitingForMachinesToStop, msg,
			corev1.ConditionFalse, logger)
		if changed {
			cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForMachinesToStop
			if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{RequeueAfter: stateCheckInterval}, nil
	}

	logger.Info("Cluster has stopped and is in hibernating state")
	changed := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonHibernating,
		"Cluster is stopped", corev1.ConditionTrue, logger)
	if changed {
		cd.Status.PowerState = hivev1.ClusterPowerStateHibernating
		if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
			return reconcile.Result{}, err
		}
		// logging with time since ready condition was set to StoppingOrHibernating state
		logCumulativeMetric(hivemetrics.MetricClusterHibernationTransitionSeconds, cd, hivev1.ClusterReadyCondition, logger)
		// Clear entry from currently stopping and waiting for cluster operators clusters if exists
		deleteTransitionMetric(hivemetrics.MetricStoppingClustersSeconds, cd)
		deleteTransitionMetric(hivemetrics.MetricWaitingForCOClustersSeconds, cd)
	}
	return reconcile.Result{}, nil
}

func (r *hibernationReconciler) checkClusterRunning(cd *hivev1.ClusterDeployment, syncSetsApplied bool, logger log.FieldLogger,
	readyCondition *hivev1.ClusterDeploymentCondition) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to check machine status")
		return reconcile.Result{}, nil
	}

	running, remaining, err := actuator.MachinesRunning(cd, r.Client, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether machines are running.")
		return reconcile.Result{}, err
	}
	if !running {
		// Ensure all machines have been started. Should have been handled already but we've seen VMs left in stopped state.
		if err := actuator.StartMachines(cd, r.Client, logger); err != nil {
			logger.WithError(err).Error("error starting machines")
			return reconcile.Result{}, err
		}

		sort.Strings(remaining) // we want to make sure the message is stable.
		msg := fmt.Sprintf("Waiting for cluster machines to start. Some machines are not yet running: %s (step 1/4)", strings.Join(remaining, ","))
		rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonWaitingForMachines, msg,
			corev1.ConditionFalse, logger)
		if rChanged {
			cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForMachines
			if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{RequeueAfter: stateCheckInterval}, nil
	}

	remoteClient, err := r.remoteClientBuilder(cd).Build()
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to connect to target cluster")
		// Special case: it's possible to get here when we're in StartingMachines state. But MachinesRunning
		// returned true, so really we're waiting for nodes. So make sure that state is set.
		if cd.Status.PowerState == hivev1.ClusterPowerStateStartingMachines {
			r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonWaitingForNodes,
				"Waiting for Nodes to be ready (step 2/4)", corev1.ConditionFalse, logger)
			cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForNodes
			if lerr := r.updateClusterDeploymentStatus(cd, logger); lerr != nil {
				return reconcile.Result{}, lerr
			}
		}
		// Regardless, return the error from remoteClientBuilder
		return reconcile.Result{}, err
	}

	preemptibleActuator, ok := actuator.(HibernationPreemptibleMachines)
	if ok {
		replaced, err := preemptibleActuator.ReplaceMachines(cd, remoteClient, logger)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to replace Preemptible machines")
			return reconcile.Result{}, err
		}
		if replaced {
			// when machines were replaced we must give some time before new nodes
			// appear.
			return reconcile.Result{RequeueAfter: nodeCheckWaitTime}, nil
		}
	}

	nodesReady, err := r.nodesReady(cd, syncSetsApplied, remoteClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether nodes are ready")
		return reconcile.Result{}, err
	}
	if !nodesReady {
		logger.Info("Nodes are not ready, checking for CSRs to approve")
		rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonWaitingForNodes,
			"Waiting for Nodes to be ready (step 2/4)", corev1.ConditionFalse, logger)
		if rChanged {
			cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForNodes
			if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
				return reconcile.Result{}, err
			}
		}
		return r.checkCSRs(cd, remoteClient, logger)
	}

	checkClusterOperators := true
	if skip, err := strconv.ParseBool(cd.Labels[constants.ResumeSkipsClusterOperatorsLabel]); err == nil {
		checkClusterOperators = !skip
	}

	if checkClusterOperators {
		// Delicate state transitions ahead. If we've now cleared the nodes phase, transition to a pause state
		// so we can give ClusterOperators time to get their pods running, before we check their status. This is
		// to avoid prematurely checking and getting good status, but from before hibernation.
		switch readyCondition.Reason {
		case hivev1.ReadyReasonStartingMachines, hivev1.ReadyReasonWaitingForMachines, hivev1.ReadyReasonWaitingForNodes:
			// Assume that if we encounter any of the previous possible states, we are ready to proceed.
			r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonPausingForClusterOperatorsToSettle,
				fmt.Sprintf("Pausing %s for ClusterOperators to settle (step 3/4)", constants.ClusterOperatorSettlePause), corev1.ConditionFalse, logger)
			cd.Status.PowerState = hivev1.ClusterPowerStatePausingForClusterOperatorsToSettle
			err := r.updateClusterDeploymentStatus(cd, logger)
			return reconcile.Result{RequeueAfter: constants.ClusterOperatorSettlePause}, err
		case hivev1.ReadyReasonPausingForClusterOperatorsToSettle:
			// Make sure we wait long enough for operators to start/settle:
			if time.Since(readyCondition.LastProbeTime.Time) < constants.ClusterOperatorSettlePause {
				remainingPause := constants.ClusterOperatorSettlePause - time.Since(readyCondition.LastProbeTime.Time)
				logger.WithField("timeRemaining", remainingPause).Info("still waiting for ClusterOperators to settle")
				return reconcile.Result{RequeueAfter: remainingPause}, nil
			}
		}

		operatorsReady, err := r.operatorsReady(remoteClient, logger)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether ClusterOperators are ready")
			return reconcile.Result{}, err
		}
		if !operatorsReady {
			logger.Info("ClusterOperators are not ready")
			rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonWaitingForClusterOperators,
				"Waiting for ClusterOperators to be ready (step 4/4)", corev1.ConditionFalse, logger)
			if rChanged {
				cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForClusterOperators
				if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
					return reconcile.Result{}, err
				}
			}
			return reconcile.Result{RequeueAfter: clusterOperatorCheckInterval}, nil
		}
	} else {
		logger.Warn("Skipping ClusterOperator health checks!")
	}

	logger.Info("Cluster has started and is in Running state")
	rChanged := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonRunning, clusterRunningMsg,
		corev1.ConditionTrue, logger)
	if rChanged {
		cd.Status.PowerState = hivev1.ClusterPowerStateRunning
		if err := r.updateClusterDeploymentStatus(cd, logger); err != nil {
			return reconcile.Result{}, err
		}
		// logging with time since hibernating condition was set to ResumingOrRunning state
		logCumulativeMetric(hivemetrics.MetricClusterReadyTransitionSeconds, cd, hivev1.ClusterHibernatingCondition, logger)
		// Clear entry from currently resuming clusters if exists
		deleteTransitionMetric(hivemetrics.MetricResumingClustersSeconds, cd)
	}
	return reconcile.Result{}, nil
}

// deleteTransitionMetric has the knowledge of labels needed to clear the metrics for currently transitioning clusters
// Note: do not use for cumulative metrics - they have different labels and are not designed to be cleared
func deleteTransitionMetric(metric *prometheus.HistogramVec, cd *hivev1.ClusterDeployment) {
	poolNS := "<none>"
	if cd.Spec.ClusterPoolRef != nil {
		poolNS = cd.Spec.ClusterPoolRef.Namespace
	}
	metric.Delete(map[string]string{
		"cluster_deployment_namespace": cd.Namespace,
		"cluster_deployment":           cd.Name,
		"platform":                     cd.Labels[hivev1.HiveClusterPlatformLabel],
		"cluster_version":              cd.Labels[constants.VersionMajorMinorPatchLabel],
		"cluster_pool_namespace":       poolNS,
	})
}

// logCumulativeMetric should be used to log cumulative metrics
// Time to log is calculated as the time lapsed since the last transition time of condition mentioned
func logCumulativeMetric(metric *prometheus.HistogramVec, cd *hivev1.ClusterDeployment,
	conditionType hivev1.ClusterDeploymentConditionType, logger log.FieldLogger) {
	condition := controllerutils.FindCondition(cd.Status.Conditions, conditionType)
	// This shouldn't happen if conditions have been properly initialized
	if condition == nil {
		logger.Warningf("cannot find %s condition, logging of metric skipped", conditionType)
		return
	}
	time := time.Since(condition.LastTransitionTime.Time).Seconds()
	if !hivemetrics.ShouldLogHistogramDurationMetric(metric, time) {
		return
	}
	poolNS, poolName := "<none>", "<none>"
	if cd.Spec.ClusterPoolRef != nil {
		poolNS = cd.Spec.ClusterPoolRef.Namespace
		poolName = cd.Spec.ClusterPoolRef.PoolName
	}
	metric.WithLabelValues(cd.Labels[constants.VersionMajorMinorPatchLabel],
		cd.Labels[hivev1.HiveClusterPlatformLabel],
		poolNS,
		poolName).Observe(time)
}

func (r *hibernationReconciler) setCDCondition(cd *hivev1.ClusterDeployment, cond hivev1.ClusterDeploymentConditionType,
	reason, message string, status corev1.ConditionStatus, logger log.FieldLogger) bool {
	changed := false
	cd.Status.Conditions, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		cond,
		status,
		reason,
		controllerutils.ErrorScrub(errors.New(message)),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
		logger.WithField("reason", reason).WithField("condition", cond).Info("condition updated on cluster deployment")
	}
	return changed
}

func (r *hibernationReconciler) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update clusterdeployment")
		return errors.Wrap(err, "failed to update clusterdeployment")
	}
	return nil
}

func (r *hibernationReconciler) getActuator(cd *hivev1.ClusterDeployment) HibernationActuator {
	for _, a := range actuators {
		if a.CanHandle(cd) {
			return a
		}
	}
	return nil
}

func (r *hibernationReconciler) hibernationSupported(cd *hivev1.ClusterDeployment) (bool, string) {
	if r.getActuator(cd) == nil {
		return false, "Unsupported platform: no actuator to handle it"
	}
	return true, "Hibernation capable"
}

func (r *hibernationReconciler) nodesReady(cd *hivev1.ClusterDeployment, syncSetsApplied bool, remoteClient client.Client, logger log.FieldLogger) (bool, error) {

	hibernatingCondition := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)
	if hibernatingCondition == nil {
		return false, errors.New("cannot find hibernating condition")
	}
	// Don't delay nodeCheckWaitTime if SyncSets have been applied
	if !syncSetsApplied && time.Since(hibernatingCondition.LastProbeTime.Time) < nodeCheckWaitTime {
		return false, nil
	}
	nodeList := &corev1.NodeList{}
	err := remoteClient.List(context.TODO(), nodeList)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to fetch cluster nodes")
		err = errors.Wrap(err, "failed to fetch cluster nodes")
		return false, err
	}
	if len(nodeList.Items) == 0 {
		logger.Info("Cluster is not reporting any nodes, waiting")
		return false, nil
	}
	for i := range nodeList.Items {
		if !isNodeReady(&nodeList.Items[i]) {
			logger.WithField("node", nodeList.Items[i].Name).Info("Node is not yet ready, waiting")
			return false, nil
		}
	}
	logger.WithField("count", len(nodeList.Items)).Info("All cluster nodes are ready")
	return true, nil
}

func (r *hibernationReconciler) operatorsReady(remoteClient client.Client, logger log.FieldLogger) (bool, error) {
	logger.Debug("Checking if ClusterOperators are ready")
	coList := &configv1.ClusterOperatorList{}
	err := remoteClient.List(context.TODO(), coList)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to fetch ClusterOperators")
		err = errors.Wrap(err, "failed to fetch ClusterOperators")
		return false, err
	}
	success := true

	for _, co := range coList.Items {
		for _, cosc := range co.Status.Conditions {
			if cosc.Type == "Disabled" && cosc.Status == "True" {
				continue
			}

			// Check that ClusterOperators are in a good state before we consider a cluster ready:
			if (cosc.Type == "Available" && cosc.Status == configv1.ConditionFalse) ||
				(cosc.Type == "Progressing" && cosc.Status == configv1.ConditionTrue) ||
				(cosc.Type == "Degraded" && cosc.Status == configv1.ConditionTrue) {
				logger.WithFields(log.Fields{
					"clusterOperator": co.Name,
					"condition":       cosc.Type,
					"status":          cosc.Status,
				}).Info("ClusterOperator is in undesired state")
				success = false
			}
		}
	}
	return success, nil
}

func (r *hibernationReconciler) checkCSRs(cd *hivev1.ClusterDeployment, remoteClient client.Client, logger log.FieldLogger) (reconcile.Result, error) {
	kubeClient, err := r.remoteClientBuilder(cd).BuildKubeClient()
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to get kube client to target cluster")
		return reconcile.Result{}, errors.Wrap(err, "failed to get kube client to target cluster")
	}
	machineList := &machineapi.MachineList{}
	err = remoteClient.List(context.TODO(), machineList)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to list machines")
		return reconcile.Result{}, errors.Wrap(err, "failed to list machines")
	}
	csrList, err := kubeClient.CertificatesV1().CertificateSigningRequests().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to list CSRs")
		return reconcile.Result{}, errors.Wrap(err, "failed to list CSRs")
	}
	for i := range csrList.Items {
		csr := &csrList.Items[i]
		csrLogger := logger.WithField("csr", csr.Name)
		if r.csrUtil.IsApproved(csr) {
			csrLogger.Debug("CSR is already approved")
			continue
		}
		parsedCSR, err := r.csrUtil.Parse(csr)
		if err != nil {
			csrLogger.WithError(err).Log(controllerutils.LogLevel(err), "failed to parse CSR")
			return reconcile.Result{}, errors.Wrap(err, "failed to parse CSR")
		}
		if err := r.csrUtil.Authorize(
			machineList.Items,
			kubeClient,
			csr,
			parsedCSR); err != nil {
			csrLogger.WithError(err).Log(controllerutils.LogLevel(err), "CSR authorization failed")
			continue
		}
		if err = r.csrUtil.Approve(kubeClient, &csrList.Items[i]); err != nil {
			csrLogger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to approve CSR")
			continue
		}
		csrLogger.Info("CSR approved")
	}
	// Requeue quickly after so we can recheck whether more CSRs need to be approved
	// TODO: remove wait time if all CSRs are approved
	return reconcile.Result{RequeueAfter: csrCheckInterval}, nil
}

func isNodeReady(node *corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// shouldStopMachines decides if machines should be stopped
func shouldStopMachines(cd *hivev1.ClusterDeployment, hibernatingCondition *hivev1.ClusterDeploymentCondition) bool {
	if cd.Spec.PowerState != hivev1.ClusterPowerStateHibernating {
		return false
	}
	if hibernatingCondition.Status == corev1.ConditionTrue {
		return false
	}
	if hibernatingCondition.Status == corev1.ConditionFalse &&
		(hibernatingCondition.Reason == hivev1.HibernatingReasonUnsupported ||
			hibernatingCondition.Reason == hivev1.HibernatingReasonStopping ||
			hibernatingCondition.Reason == hivev1.HibernatingReasonWaitingForMachinesToStop) {
		return false
	}
	return true
}

// shouldStartMachines decides if machines should be started
func shouldStartMachines(cd *hivev1.ClusterDeployment, hibernatingCondition *hivev1.ClusterDeploymentCondition,
	readyCondition *hivev1.ClusterDeploymentCondition) bool {
	if cd.Spec.PowerState == hivev1.ClusterPowerStateHibernating {
		return false
	}
	if readyCondition.Status == corev1.ConditionTrue {
		return false
	}
	if readyCondition.Reason == hivev1.ReadyReasonStartingMachines {
		return false
	}
	if hibernatingCondition.Status == corev1.ConditionFalse &&
		(hibernatingCondition.Reason != hivev1.HibernatingReasonSyncSetsApplied) {
		// reason is either ResumingOrRunning, FailedToStop, Unsupported or SyncSetsNotApplied
		return false
	}
	return true
}

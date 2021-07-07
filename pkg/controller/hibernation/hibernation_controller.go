package hibernation

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
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

	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

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

	// nodeCheckWaitTime is the minimum time to wait for a node
	// ready check after a cluster started resuming. This is to
	// avoid a false positive when the node status is checked too
	// soon after the cluster is ready
	nodeCheckWaitTime = 4 * time.Minute

	// hibernateAfterSyncSetsNotApplied is the amount of time to wait
	// before hibernating when SyncSets have not been applied
	hibernateAfterSyncSetsNotApplied = 10 * time.Minute
)

var (
	// minimumClusterVersion is the minimum supported version for
	// hibernation
	minimumClusterVersion = semver.MustParse("4.4.8")

	// actuators is a list of available actuators for this controller
	// It is populated via the RegisterActuator function
	actuators []HibernationActuator

	// clusterDeploymentHibernationConditions are the cluster deployment conditions controlled by
	// hibernation controller
	clusterDeploymentHibernationConditions = []hivev1.ClusterDeploymentConditionType{
		hivev1.ClusterHibernatingCondition,
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
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Log(controllerutils.LogLevel(err), "Error creating controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
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

	// If cluster is already deleted, skip any processing
	if !cd.DeletionTimestamp.IsZero() {
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions, clusterDeploymentHibernationConditions)
	if len(newConditions) > len(cd.Status.Conditions) {
		cd.Status.Conditions = newConditions
		cdLog.Info("initializing hibernating controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
	}

	// If cluster is not installed, skip any processing
	if !cd.Spec.Installed {
		return reconcile.Result{}, nil
	}

	// set hibernating condition to false for unsupported clouds
	if supported, msg := r.hibernationSupported(cd); !supported {
		return r.setHibernatingCondition(cd, hivev1.UnsupportedHibernationReason, msg, corev1.ConditionFalse, cdLog)
	}

	shouldHibernate := cd.Spec.PowerState == hivev1.HibernatingClusterPowerState
	hibernatingCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)

	// Signal a problem if we should be hibernating or have requested hibernate after and the
	// SyncSets have not yet been applied.
	if shouldHibernate || cd.Spec.HibernateAfter != nil {
		// Determine if SyncSets have been applied
		clusterSync := &hiveintv1alpha1.ClusterSync{}
		err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, clusterSync)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("could not get ClusterSync: %v", err)
		}
		// SyncSets have not been applied
		if clusterSync.Status.FirstSuccessTime == nil {
			// Allow hibernation (do not set condition) if hibernateAfterSyncSetsNotApplied duration has passed since cluster
			// installed and syncsets still not applied
			if cd.Status.InstalledTimestamp != nil && time.Now().Sub(cd.Status.InstalledTimestamp.Time) < hibernateAfterSyncSetsNotApplied {
				return r.setHibernatingCondition(cd, hivev1.SyncSetsNotAppliedReason, "Cluster SyncSets have not been applied", corev1.ConditionFalse, cdLog)
			}
		}
	}

	// Clear any lingering unsupported hibernation condition
	if hibernatingCondition.Reason == hivev1.UnsupportedHibernationReason {
		if supported, msg := r.hibernationSupported(cd); supported {
			return r.setHibernatingCondition(cd, hivev1.RunningHibernationReason, msg, corev1.ConditionFalse, cdLog)
		}
	}

	// Check if HibernateAfter is set, and if the cluster has been in running state for longer than this duration, put it to sleep.
	if cd.Spec.HibernateAfter != nil && cd.Spec.PowerState != hivev1.HibernatingClusterPowerState {
		hibernateAfterDur := cd.Spec.HibernateAfter.Duration
		runningSince := cd.Status.InstalledTimestamp.Time
		hibLog := cdLog.WithFields(log.Fields{
			"runningSince":   runningSince,
			"hibernateAfter": hibernateAfterDur,
		})
		var isRunning bool

		cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)
		if cond.Status == corev1.ConditionUnknown {
			hibLog.Debug("cluster has never been hibernated, using installed time")
			isRunning = true
		} else if cond.Status == corev1.ConditionFalse {
			runningSince = cond.LastTransitionTime.Time
			hibLog = hibLog.WithField("runningSince", runningSince)
			hibLog.WithField("reason", cond.Reason).Debug("hibernating condition false")
			isRunning = true
		}

		if isRunning {
			expiry := runningSince.Add(hibernateAfterDur)
			hibLog.Debugf("cluster should be hibernating after: %s", expiry)
			if time.Now().After(expiry) {
				hibLog.WithField("expiry", expiry).Debug("cluster has been running longer than hibernate-after duration, moving to hibernating powerState")
				cd.Spec.PowerState = hivev1.HibernatingClusterPowerState
				if controllerutils.IsFakeCluster(cd) {
					controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions,
						hivev1.ClusterHibernatingCondition,
						corev1.ConditionTrue,
						hivev1.HibernatingHibernationReason,
						"Fake cluster is stopped",
						controllerutils.UpdateConditionNever)
				}
				err := r.Update(context.TODO(), cd)
				if err != nil {
					hibLog.WithError(err).Log(controllerutils.LogLevel(err), "error hibernating cluster")
				}
				return reconcile.Result{}, err
			}

			defer func() {
				requeueNow := result.Requeue && result.RequeueAfter <= 0
				if returnErr == nil && !requeueNow {
					// We have an hibernate after time but cluster has not been running that long yet.
					// Set requeueAfter for just after so that we requeue cluster for hibernation once reconcile has completed
					requeueAfter := time.Until(expiry)
					if requeueAfter < result.RequeueAfter || result.RequeueAfter <= 0 {
						hibLog.Infof("cluster will reconcile due to hibernate-after time in: %v", requeueAfter)
						result.RequeueAfter = requeueAfter
						result.Requeue = true
					}
				}
			}()
		}

	}

	if !shouldHibernate {
		if hibernatingCondition.Status == corev1.ConditionUnknown || hibernatingCondition.Status == corev1.ConditionFalse {
			return reconcile.Result{}, nil
		}
		switch hibernatingCondition.Reason {
		case hivev1.StoppingHibernationReason, hivev1.HibernatingHibernationReason, hivev1.FailedToStartHibernationReason:
			if controllerutils.IsFakeCluster(cd) {
				cd.Spec.PowerState = hivev1.RunningClusterPowerState
				if controllerutils.IsFakeCluster(cd) {
					controllerutils.SetClusterDeploymentCondition(cd.Status.Conditions,
						hivev1.ClusterHibernatingCondition,
						corev1.ConditionFalse,
						hivev1.RunningHibernationReason,
						"Fake cluster is running",
						controllerutils.UpdateConditionNever)
				}
				err := r.Update(context.TODO(), cd)
				if err != nil {
					cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error resuming cluster")
				}
				return reconcile.Result{}, err
			}
			return r.startMachines(cd, cdLog)
		case hivev1.ResumingHibernationReason:
			return r.checkClusterResumed(cd, cdLog)
		}
		return reconcile.Result{}, nil
	}

	if (hibernatingCondition.Status == corev1.ConditionUnknown || hibernatingCondition.Status == corev1.ConditionFalse &&
		hibernatingCondition.Reason != hivev1.UnsupportedHibernationReason) ||
		hibernatingCondition.Reason == hivev1.ResumingHibernationReason ||
		(cd.Spec.PowerState == hivev1.HibernatingClusterPowerState && hibernatingCondition.Reason == hivev1.FailedToStartHibernationReason) {
		return r.stopMachines(cd, cdLog)
	}
	if hibernatingCondition.Reason == hivev1.StoppingHibernationReason {
		return r.checkClusterStopped(cd, false, cdLog)
	}
	return reconcile.Result{}, nil
}

func (r *hibernationReconciler) startMachines(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to start cluster machines")
		return reconcile.Result{}, nil
	}
	logger.Info("Resuming cluster")
	if err := actuator.StartMachines(cd, r.Client, logger); err != nil {
		msg := fmt.Sprintf("Failed to start machines: %v", err)
		result, condErr := r.setHibernatingCondition(cd, hivev1.FailedToStartHibernationReason, msg, corev1.ConditionTrue, logger)
		if condErr != nil {
			return reconcile.Result{}, condErr
		}
		// Return the error starting machines so we get requeue + backoff
		return result, err
	}
	return r.setHibernatingCondition(cd, hivev1.ResumingHibernationReason, "Starting cluster machines", corev1.ConditionTrue, logger)
}

func (r *hibernationReconciler) stopMachines(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	actuator := r.getActuator(cd)
	if actuator == nil {
		logger.Warning("No compatible actuator found to start cluster machines")
		return reconcile.Result{}, nil
	}
	logger.Info("Stopping cluster")
	if err := actuator.StopMachines(cd, r.Client, logger); err != nil {
		msg := fmt.Sprintf("Failed to stop machines: %v", err)
		return r.setHibernatingCondition(cd, hivev1.FailedToStopHibernationReason, msg, corev1.ConditionFalse, logger)
	}
	return r.setHibernatingCondition(cd, hivev1.StoppingHibernationReason, "Stopping cluster machines", corev1.ConditionTrue, logger)
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
		_, condErr := r.setHibernatingCondition(cd, hivev1.StoppingHibernationReason, msg, corev1.ConditionTrue, logger)
		if condErr != nil {
			return reconcile.Result{}, condErr
		}

		return reconcile.Result{RequeueAfter: stateCheckInterval}, nil
	}

	logger.Info("Cluster has stopped and is in hibernating state")
	return r.setHibernatingCondition(cd, hivev1.HibernatingHibernationReason, "Cluster is stopped", corev1.ConditionTrue, logger)
}

func (r *hibernationReconciler) checkClusterResumed(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
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
		msg := fmt.Sprintf("Starting cluster machines. Some machines are not yet running: %s", strings.Join(remaining, ","))
		_, condErr := r.setHibernatingCondition(cd, hivev1.ResumingHibernationReason, msg, corev1.ConditionTrue, logger)
		if condErr != nil {
			return reconcile.Result{}, condErr
		}

		return reconcile.Result{RequeueAfter: stateCheckInterval}, nil
	}

	remoteClient, err := r.remoteClientBuilder(cd).Build()
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to connect to target cluster")
		return reconcile.Result{}, err
	}
	ready, err := r.nodesReady(cd, remoteClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether nodes are ready")
		return reconcile.Result{}, err
	}
	if !ready {
		logger.Info("Nodes are not ready, checking for CSRs to approve")
		return r.checkCSRs(cd, remoteClient, logger)
	}
	logger.Info("Cluster has started and is in Running state")
	return r.setHibernatingCondition(cd, hivev1.RunningHibernationReason, "All machines are started and nodes are ready", corev1.ConditionFalse, logger)
}

func (r *hibernationReconciler) setHibernatingCondition(cd *hivev1.ClusterDeployment, reason, message string, status corev1.ConditionStatus, logger log.FieldLogger) (result reconcile.Result, returnErr error) {
	changed := false
	cd.Status.Conditions, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.ClusterHibernatingCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)

	if reason == hivev1.SyncSetsNotAppliedReason {
		defer func() {
			expiry := cd.Status.InstalledTimestamp.Time.Add(hibernateAfterSyncSetsNotApplied)
			requeueAfter := time.Until(expiry)
			logger.Infof("cluster will reconcile due to syncsets not applied in: %v", requeueAfter)
			result.RequeueAfter = requeueAfter
			result.Requeue = true
			returnErr = errors.New(message)
		}()
	}

	if changed {
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to update hibernating condition")
			return reconcile.Result{}, errors.Wrap(err, "failed to update hibernating condition")
		}
		logger.WithField("reason", reason).Info("Hibernating condition updated on cluster deployment.")
	}
	return reconcile.Result{}, nil
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
	versionString, versionPresent := cd.Labels[constants.VersionMajorMinorPatchLabel]
	if !versionPresent {
		return false, "No cluster version is available yet"
	}
	version, err := semver.Parse(versionString)
	if err != nil {
		return false, fmt.Sprintf("Cannot parse cluster version: %v", err)
	}
	if version.LT(minimumClusterVersion) {
		return false, fmt.Sprintf("Unsupported version, need version %s or greater", minimumClusterVersion.String())
	}
	return true, "Hibernation capable"
}

func (r *hibernationReconciler) nodesReady(cd *hivev1.ClusterDeployment, remoteClient client.Client, logger log.FieldLogger) (bool, error) {

	hibernatingCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)
	if hibernatingCondition == nil {
		return false, errors.New("cannot find hibernating condition")
	}
	if time.Since(hibernatingCondition.LastProbeTime.Time) < nodeCheckWaitTime {
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
	csrList, err := kubeClient.CertificatesV1beta1().CertificateSigningRequests().List(context.TODO(), metav1.ListOptions{})
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

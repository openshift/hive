package hibernation

import (
	"context"
	"fmt"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	// ControllerName is the name of this controller
	ControllerName = "hibernation"

	// stateCheckInterval is the time interval for polling
	// whether a cluster's machines are stopped or are running
	stateCheckInterval = 60 * time.Second

	// csrCheckInterval is the time interval for polling
	// pending CSRs
	csrCheckInterval = 30 * time.Second

	// nodeCheckWaitTime is the minimum time to wait for a node
	// ready check after a cluster started resuming. This is to
	// avoid a false positive when the node status is checked too
	// soon after the cluster is ready
	nodeCheckWaitTime = 2 * time.Minute
)

var (
	// minimumClusterVersion is the minimum supported version for
	// hibernation
	minimumClusterVersion = semver.MustParse("4.4.8")

	// actuators is a list of available actuators for this controller
	// It is populated via the RegisterActuator function
	actuators []HibernationActuator
)

// Add creates a new Hibernation controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
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
func NewReconciler(mgr manager.Manager) *hibernationReconciler {
	logger := log.WithField("controller", ControllerName)
	r := &hibernationReconciler{
		Client:  controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		logger:  logger,
		csrUtil: &csrUtility{},
	}
	r.remoteClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// AddToManager adds a new Controller to the controller manager
func AddToManager(mgr manager.Manager, r *hibernationReconciler) error {
	c, err := controller.New("hibernation-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
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
func (r *hibernationReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"controller":        ControllerName,
		"clusterDeployment": request.NamespacedName.String(),
	})

	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

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

	// If cluster is not installed, skip any processing
	if !cd.Spec.Installed {
		return reconcile.Result{}, nil
	}

	shouldHibernate := cd.Spec.PowerState == hivev1.HibernatingClusterPowerState
	hibernatingCondition := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition)

	if !shouldHibernate {
		if hibernatingCondition == nil || hibernatingCondition.Status == corev1.ConditionFalse {
			return reconcile.Result{}, nil
		}
		switch hibernatingCondition.Reason {
		case hivev1.StoppingHibernationReason, hivev1.HibernatingHibernationReason:
			return r.startMachines(cd, cdLog)
		case hivev1.ResumingHibernationReason:
			return r.checkClusterResumed(cd, cdLog)
		}
		return reconcile.Result{}, nil
	}

	if supported, msg := r.canHibernate(cd); !supported {
		return r.setHibernatingCondition(cd, hivev1.UnsupportedHibernationReason, msg, corev1.ConditionFalse, cdLog)
	}
	if hibernatingCondition == nil || hibernatingCondition.Status == corev1.ConditionFalse || hibernatingCondition.Reason == hivev1.ResumingHibernationReason {
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
		return r.setHibernatingCondition(cd, hivev1.FailedToStartHibernationReason, msg, corev1.ConditionTrue, logger)
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
	stopped, err := actuator.MachinesStopped(cd, r.Client, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether machines are stopped.")
		return reconcile.Result{}, err
	}
	if !stopped {
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
	running, err := actuator.MachinesRunning(cd, r.Client, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to check whether machines are running.")
		return reconcile.Result{}, err
	}
	if !running {
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

func (r *hibernationReconciler) setHibernatingCondition(cd *hivev1.ClusterDeployment, reason, message string, status corev1.ConditionStatus, logger log.FieldLogger) (reconcile.Result, error) {
	changed := false
	if status == corev1.ConditionFalse && controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition) == nil {
		now := metav1.Now()
		cd.Status.Conditions = append(cd.Status.Conditions, hivev1.ClusterDeploymentCondition{
			Type:               hivev1.ClusterHibernatingCondition,
			Status:             corev1.ConditionFalse,
			Reason:             reason,
			Message:            message,
			LastProbeTime:      now,
			LastTransitionTime: now,
		})
		changed = true
	} else {
		cd.Status.Conditions, changed = controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.ClusterHibernatingCondition,
			status,
			reason,
			message,
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
	}
	if changed {
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to update hibernating condition")
			return reconcile.Result{}, errors.Wrap(err, "failed to update hibernating condition")
		}
		logger.Info("Hibernating condition updated on cluster deployment.")
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

func (r *hibernationReconciler) canHibernate(cd *hivev1.ClusterDeployment) (bool, string) {
	if r.getActuator(cd) == nil {
		return false, "Unsupported platform: no actuator to handle it"
	}
	if cd.Status.ClusterVersionStatus == nil || len(cd.Status.ClusterVersionStatus.Desired.Version) == 0 {
		return false, "No cluster version is available yet"
	}
	version, err := semver.Parse(cd.Status.ClusterVersionStatus.Desired.Version)
	if err != nil {
		return false, fmt.Sprintf("Cannot parse cluster version: %v", err)
	}
	if version.LT(minimumClusterVersion) {
		return false, fmt.Sprintf("Unsupported version, need version %s or greater", minimumClusterVersion.String())
	}
	return true, ""
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

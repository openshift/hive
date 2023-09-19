package fakeclusterinstall

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	librarygocontroller "github.com/openshift/library-go/pkg/controller"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveint "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = hivev1.FakeClusterInstallControllerName
)

// Add creates a new FakeClusterInstall controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	r := &ReconcileClusterInstall{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", ControllerName),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	c, err := controller.New("fakeclusterinstall-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error creating new fakeclusterinstall controller")
		return err
	}

	// Watch for changes to FakeClusterInstall
	err = c.Watch(source.Kind(mgr.GetCache(), &hiveint.FakeClusterInstall{}), &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("Error watching FakeClusterInstall")
		return err
	}

	// TODO: also watch for changes to ClusterDeployment? Agent installs try to respond to changes there as well.

	return nil
}

// ReconcileClusterInstall is the reconciler for FakeClusterInstall.
type ReconcileClusterInstall struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger
}

// Reconcile ensures that a given FakeClusterInstall resource exists and reflects the state of cluster operators from its target cluster
func (r *ReconcileClusterInstall) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "fakeClusterInstall", request.NamespacedName)
	logger.Info("reconciling FakeClusterInstall")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the FakeClusterInstall instance
	fci := &hiveint.FakeClusterInstall{}
	err := r.Get(context.TODO(), request.NamespacedName, fci)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logger.Debug("FakeClusterInstall not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.WithError(err).Error("Error getting FakeClusterInstall")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: fci}, logger)

	if !fci.DeletionTimestamp.IsZero() {
		logger.Info("FakeClusterInstall resource has been deleted")
		return reconcile.Result{}, nil
	}

	// Ensure our conditions are present, default state should be Unknown per Kube guidelines:
	conditionTypes := []hivev1.ClusterInstallConditionType{
		// These conditions are required by Hive:
		hivev1.ClusterInstallCompleted,
		hivev1.ClusterInstallFailed,
		hivev1.ClusterInstallStopped,
		hivev1.ClusterInstallRequirementsMet,
	}

	var anyChanged bool
	for _, condType := range conditionTypes {
		c := controllerutils.FindCondition(fci.Status.Conditions, condType)
		if c == nil {
			logger.WithField("condition", condType).Info("initializing condition with Unknown status")
			newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
				fci.Status.Conditions,
				condType,
				corev1.ConditionUnknown,
				"",
				"",
				controllerutils.UpdateConditionAlways)
			fci.Status.Conditions = newConditions
			anyChanged = anyChanged || changed
		}
	}

	if anyChanged {
		err := updateClusterInstallStatus(r.Client, fci, logger)
		return reconcile.Result{}, err
	}

	// Fetch corresponding ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	switch err = r.Get(context.TODO(), request.NamespacedName, cd); {
	case apierrors.IsNotFound(err):
		// TODO: assuming same name, add explicit reference of some kind between cluster install and cluster deplopyment
		logger.WithField("clusterDeployment", request.NamespacedName).Info("ClusterDeployment not found")
		return reconcile.Result{}, nil
	case err != nil:
		logger.WithError(err).Error("Error getting ClusterDeployment")
		return reconcile.Result{}, err
	}
	if !cd.DeletionTimestamp.IsZero() {
		logger.Debug("ClusterDeployment has been deleted")
		return reconcile.Result{}, nil
	}

	// Ensure the FakeClusterInstall has an OwnerReference to the ClusterDeployment, so it is
	// automatically cleaned up if the owner is deleted.
	cdRef := metav1.OwnerReference{
		APIVersion:         cd.APIVersion,
		Kind:               cd.Kind,
		Name:               cd.Name,
		UID:                cd.UID,
		BlockOwnerDeletion: pointer.BoolPtr(true),
	}
	cdRefChanged := librarygocontroller.EnsureOwnerRef(fci, cdRef)
	if cdRefChanged {
		logger.Info("added owner reference to ClusterDeployment")
		return reconcile.Result{}, r.Update(context.TODO(), fci)
	}

	// Check if we're Completed and can exit reconcile early.
	completedCond := controllerutils.FindCondition(fci.Status.Conditions, hivev1.ClusterInstallCompleted)
	if completedCond.Status == corev1.ConditionTrue {
		// Ensure Stopped=True
		newConditions, changedStopped := controllerutils.SetClusterInstallConditionWithChangeCheck(
			fci.Status.Conditions,
			hivev1.ClusterInstallStopped,
			corev1.ConditionTrue,
			"ClusterInstalled",
			"Cluster install completed successfully",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		// Ensure Failed=False
		newConditions, changedFailed := controllerutils.SetClusterInstallConditionWithChangeCheck(
			newConditions,
			hivev1.ClusterInstallFailed,
			corev1.ConditionFalse,
			"ClusterInstalled",
			"Cluster install completed successfully",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changedStopped || changedFailed {
			fci.Status.Conditions = newConditions
			err := updateClusterInstallStatus(r.Client, fci, logger)
			return reconcile.Result{}, err
		}
		logger.Info("cluster install completed, no work left to be done")
		return reconcile.Result{}, err
	}

	// NOTE: While this controller does not support a Stopped=True Completed=False state (it will try
	// forever), most real implementations would want to check if it's time to give up here.

	// Ensure Stopped=False as we are actively working to reconcile:
	newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
		fci.Status.Conditions,
		hivev1.ClusterInstallStopped,
		corev1.ConditionFalse,
		"InProgress",
		"Cluster install in progress",
		controllerutils.UpdateConditionIfReasonOrMessageChange)
	if changed {
		fci.Status.Conditions = newConditions
		err := updateClusterInstallStatus(r.Client, fci, logger)
		return reconcile.Result{}, err
	}

	// Simulate 30 second wait for RequirementsMet condition to go True:
	reqsCond := controllerutils.FindCondition(fci.Status.Conditions, hivev1.ClusterInstallRequirementsMet)
	switch reqsCond.Status {
	case corev1.ConditionUnknown:
		logger.Info("setting RequirementsMet condition to False")
		newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
			fci.Status.Conditions,
			hivev1.ClusterInstallRequirementsMet,
			corev1.ConditionFalse,
			"WaitingForRequirements",
			"Waiting 30 seconds before considering requirements met",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			fci.Status.Conditions = newConditions
			err := updateClusterInstallStatus(r.Client, fci, logger)
			return reconcile.Result{}, err
		}
	case corev1.ConditionFalse:
		// Check if it's been 30 seconds since we set condition to False:
		delta := time.Since(reqsCond.LastTransitionTime.Time)
		if delta < 30*time.Second {
			// requeue for remainder of delta
			return reconcile.Result{RequeueAfter: 30*time.Second - delta}, nil
		}

		logger.Info("setting RequirementsMet condition to True")
		newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
			fci.Status.Conditions,
			hivev1.ClusterInstallRequirementsMet,
			corev1.ConditionTrue,
			"AllRequirementsMet",
			"All requirements met",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			fci.Status.Conditions = newConditions
			err := updateClusterInstallStatus(r.Client, fci, logger)
			return reconcile.Result{}, err
		}
	}

	// Simulate 30 second wait for Completed condition to go True:
	switch completedCond.Status {
	case corev1.ConditionUnknown:
		logger.Info("setting Completed condition to False")
		newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
			fci.Status.Conditions,
			hivev1.ClusterInstallCompleted,
			corev1.ConditionFalse,
			"InProgress",
			"Installation in progress",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			fci.Status.Conditions = newConditions
			err := updateClusterInstallStatus(r.Client, fci, logger)
			return reconcile.Result{}, err
		}
	case corev1.ConditionFalse:
		// Set ClusterMetadata if install is underway:
		if fci.Spec.ClusterMetadata == nil {
			fci.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
				ClusterID: "not-a-real-cluster",
				InfraID:   "not-a-real-cluster",
				// TODO: do we need to create dummy secrets?
				AdminKubeconfigSecretRef: corev1.LocalObjectReference{Name: "admin-kubeconfig"},
				AdminPasswordSecretRef:   &corev1.LocalObjectReference{Name: "admin-password"},
			}
			logger.Info("setting fake ClusterMetadata")
			return reconcile.Result{}, r.Client.Update(context.Background(), fci)
		}

		// Check if it's been 30 seconds since we set condition to False:
		delta := time.Since(completedCond.LastTransitionTime.Time)
		if delta < 30*time.Second {
			// requeue for remainder of delta
			return reconcile.Result{RequeueAfter: 30*time.Second - delta}, nil
		}
		logger.Info("setting Completed condition to True")
		newConditions, changed := controllerutils.SetClusterInstallConditionWithChangeCheck(
			fci.Status.Conditions,
			hivev1.ClusterInstallCompleted,
			corev1.ConditionTrue,
			"ClusterInstalled",
			"Cluster install completed successfully",
			controllerutils.UpdateConditionIfReasonOrMessageChange)
		if changed {
			fci.Status.Conditions = newConditions
			err := updateClusterInstallStatus(r.Client, fci, logger)
			return reconcile.Result{}, err
		}
	}

	logger.Info("cluster is already installed")

	return reconcile.Result{}, nil
}

func updateClusterInstallStatus(c client.Client, fci *hiveint.FakeClusterInstall, logger log.FieldLogger) error {
	// TODO: deepequals check
	logger.Info("updating status")
	return c.Status().Update(context.Background(), fci)
}

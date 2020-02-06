package clusterstate

import (
	"context"
	"fmt"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	controllerName       = "clusterState"
	statusUpdateInterval = 10 * time.Minute
)

// Add creates a new ClusterState controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterState{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:       mgr.GetScheme(),
		logger:       log.WithField("controller", controllerName),
		updateStatus: updateClusterStateStatus,
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, controllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	c, err := controller.New("clusterstate-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error creating new clusterstate controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		log.WithField("controller", controllerName).WithError(err).Error("Error watching cluster deployment")
		return err
	}
	return nil
}

// ReconcileClusterState is the reconciler for ClusterState. It will sync on ClusterDeployment resources
// and ensure that a ClusterState exists and is updated when appropriate.
type ReconcileClusterState struct {
	client.Client
	scheme *runtime.Scheme
	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder

	// updateStatus updates a given cluster state's status, exposed for testing
	updateStatus func(client.Client, *hivev1.ClusterState) error
}

// Reconcile ensures that a given ClusterState resource exists and reflects the state of cluster operators from its target cluster
func (r *ReconcileClusterState) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithFields(log.Fields{
		"controller":        controllerName,
		"clusterDeployment": request.NamespacedName.String(),
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	logger.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			logger.Debug("cluster deployment not found")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		logger.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}
	if !cd.DeletionTimestamp.IsZero() {
		logger.Debug("ClusterDeployment resource has been deleted")
		return reconcile.Result{}, nil
	}
	if !cd.Spec.Installed {
		logger.Debug("ClusterDeployment is not yet ready")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		logger.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)

	// If the cluster is unreachable, do not reconcile.
	if remoteClientBuilder.Unreachable() {
		logger.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	// Fetch corresponding ClusterState instance
	st := &hivev1.ClusterState{}
	switch err = r.Get(context.TODO(), request.NamespacedName, st); {
	case apierrors.IsNotFound(err):
		logger.Info("Creating cluster state resource for cluster deployment")
		st.Name = cd.Name
		st.Namespace = cd.Namespace

		logger.WithField("derivedObject", st.Name).Debug("Setting label on derived object")
		st.Labels = k8slabels.AddLabel(st.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
		if err = controllerutil.SetControllerReference(cd, st, r.scheme); err != nil {
			logger.WithError(err).Error("error setting controller reference on cluster state")
			return reconcile.Result{}, err
		}
		err = r.Create(context.TODO(), st)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to create cluster state")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	case err != nil:
		logger.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}
	if !st.DeletionTimestamp.IsZero() {
		logger.Debug("ClusterState resource has been deleted")
		// If the previous cluster state is getting deleted, requeue in a few seconds
		// to check again and recreate it if necessary.
		logger.Info("Waiting 60 seconds for cluster state to finish deleting")
		return reconcile.Result{RequeueAfter: 60 * time.Second}, nil
	}
	if st.Status.LastUpdated != nil {
		timeSinceLastUpdate := time.Since(st.Status.LastUpdated.Time)
		if timeSinceLastUpdate < statusUpdateInterval {
			nextUpdateWait := statusUpdateInterval - timeSinceLastUpdate
			logger.Debugf("Waiting to fetch clusteroperator status in %v", nextUpdateWait)
			return reconcile.Result{RequeueAfter: nextUpdateWait}, nil
		}
	}

	remoteClient, err := remoteClientBuilder.Build()
	if err != nil {
		logger.WithError(err).Error("error building remote cluster client connection")
		return reconcile.Result{}, err
	}
	clusterOperators := &configv1.ClusterOperatorList{}
	err = remoteClient.List(context.TODO(), clusterOperators)
	if err != nil {
		logger.WithError(err).Error("failed to list target cluster operators")
		return reconcile.Result{}, err
	}
	return r.syncOperatorStates(clusterOperators.Items, st, logger)
}

func (r *ReconcileClusterState) syncOperatorStates(operators []configv1.ClusterOperator, st *hivev1.ClusterState, logger log.FieldLogger) (reconcile.Result, error) {
	operatorStates := make([]hivev1.ClusterOperatorState, len(operators))
	for i, clusterOperator := range operators {
		operatorStates[i] = hivev1.ClusterOperatorState{
			Name:       clusterOperator.Name,
			Conditions: clusterOperator.Status.Conditions,
		}
	}
	if operatorStatesChanged(logger, st.Status.ClusterOperators, operatorStates) {
		st.Status.ClusterOperators = operatorStates
		now := metav1.Now()
		st.Status.LastUpdated = &now
		if err := r.updateStatus(r, st); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster operator state")
			return reconcile.Result{}, err
		}
		logger.Info("clusterState has been updated")
		return reconcile.Result{}, nil
	}
	return reconcile.Result{
		RequeueAfter: statusUpdateInterval,
	}, nil
}

func operatorStatesChanged(logger log.FieldLogger, existing, updated []hivev1.ClusterOperatorState) bool {
	changed := false
	existingNames := sets.NewString()
	updatedNames := sets.NewString()

	for _, i := range existing {
		existingNames.Insert(i.Name)
	}
	for _, i := range updated {
		updatedNames.Insert(i.Name)
	}

	removed := existingNames.Difference(updatedNames)
	added := updatedNames.Difference(existingNames)
	same := existingNames.Intersection(updatedNames)
	if removed.Len() > 0 {
		changed = true
		logger.Infof("Removed cluster operators: %v", removed.List())
	}
	if added.Len() > 0 {
		changed = true
		logger.Infof("Added cluster operators: %v", added.List())
	}

	for _, name := range same.List() {
		i := indexOfOperatorState(existing, name)
		j := indexOfOperatorState(updated, name)
		if singleOperatorStateChanged(logger, existing[i], updated[j]) {
			changed = true
		}
	}
	return changed
}

func singleOperatorStateChanged(logger log.FieldLogger, existing, updated hivev1.ClusterOperatorState) bool {
	changed := false
	existingTypes := sets.NewString()
	updatedTypes := sets.NewString()

	for _, c := range existing.Conditions {
		existingTypes.Insert(string(c.Type))
	}
	for _, c := range updated.Conditions {
		updatedTypes.Insert(string(c.Type))
	}
	removed := existingTypes.Difference(updatedTypes)
	added := updatedTypes.Difference(existingTypes)
	same := existingTypes.Intersection(updatedTypes)

	if removed.Len() > 0 {
		logger.Infof("Removed conditions for operator %s: %v", existing.Name, removed.List())
		changed = true
	}
	if added.Len() > 0 {
		logger.Infof("Added conditions for operator %s: %v", existing.Name, added.List())
		changed = true
	}
	for _, ctype := range same.List() {
		i := indexOfCondition(existing.Conditions, ctype)
		j := indexOfCondition(updated.Conditions, ctype)
		if operatorConditionChanged(logger, existing.Name, existing.Conditions[i], updated.Conditions[j]) {
			changed = true
		}
	}
	return changed
}

func operatorConditionChanged(logger log.FieldLogger, name string, existing, updated configv1.ClusterOperatorStatusCondition) bool {
	if reflect.DeepEqual(existing, updated) {
		return false
	}
	changeDesc := ""
	if existing.Status != updated.Status {
		changeDesc = fmt.Sprintf(" status (%s -> %s)", existing.Status, updated.Status)
	}
	logger.Infof("condition %s changed for operator %s%s", existing.Type, name, changeDesc)
	return true
}

func indexOfOperatorState(states []hivev1.ClusterOperatorState, name string) int {
	for i, state := range states {
		if state.Name == name {
			return i
		}
	}
	return -1
}

func indexOfCondition(conditions []configv1.ClusterOperatorStatusCondition, ctype string) int {
	for i, condition := range conditions {
		if string(condition.Type) == ctype {
			return i
		}
	}
	return -1
}

func updateClusterStateStatus(c client.Client, cs *hivev1.ClusterState) error {
	return c.Status().Update(context.Background(), cs)
}

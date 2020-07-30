package syncset

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/yaml"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/controller/clustersyncset"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = "syncset"
)

// Add creates a new SyncSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileSyncSet{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		logger: log.WithField("controller", ControllerName),
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("syncset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return fmt.Errorf("cannot create new syncset-controller: %v", err)
	}

	// Watch for SyncSet
	if err := c.Watch(&source.Kind{Type: &hivev1.SyncSet{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("cannot start watch on syncsets: %w", err)
	}

	// Watch for SelectorSyncSet
	if err := c.Watch(&source.Kind{Type: &hivev1.SelectorSyncSet{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return fmt.Errorf("cannot start watch on selectorsyncsets: %w", err)
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSyncSet{}

// ReconcileSyncSet reconciles a ClusterDeployment and the SyncSets associated with it
type ReconcileSyncSet struct {
	client.Client
	logger log.FieldLogger
}

// Reconcile fills out the status and ObservedGeneration for SyncSets and SelectorSyncSets
func (r *ReconcileSyncSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithField("syncSet", request.NamespacedName)

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	var syncSet clustersyncset.CommonSyncSet
	if request.Namespace == "" {
		syncSet = (*clustersyncset.SelectorSyncSetAsCommon)(&hivev1.SelectorSyncSet{})
	} else {
		syncSet = (*clustersyncset.SyncSetAsCommon)(&hivev1.SyncSet{})
	}
	err := r.Get(context.TODO(), request.NamespacedName, syncSet.AsRuntimeObject())
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		logger.WithError(err).Error("error looking up syncset")
		return reconcile.Result{}, err
	}

	// If the syncset is deleted, do not reconcile.
	if syncSet.AsMetaObject().GetDeletionTimestamp() != nil {
		logger.Debug("syncset is being deleted, nothing to do")
		return reconcile.Result{}, nil
	}

	if syncSet.AsMetaObject().GetGeneration() == syncSet.GetStatus().ObservedGeneration {
		logger.Debug("syncset generation has not changed, nothing to do")
		return reconcile.Result{}, nil
	}

	var resourceErr error
	resources := make([]hivev1.ResourceIdentification, len(syncSet.GetSpec().Resources))
	for i, res := range syncSet.GetSpec().Resources {
		logger := logger.WithField("resource", i)
		u := unstructured.Unstructured{}
		switch err := yaml.Unmarshal(res.Raw, &u); {
		case err != nil:
			logger.WithError(err).Warn("could not unmarshal resource")
			resourceErr = err
		case u.GetKind() == "":
			logger.Warn("missing Kind")
			resourceErr = fmt.Errorf("missing Kind")
		case u.GetName() == "":
			logger.Warn("missing Name")
			resourceErr = fmt.Errorf("missing Name")
		default:
			resources[i].APIVersion = u.GetAPIVersion()
			resources[i].Kind = u.GetKind()
			resources[i].Namespace = u.GetNamespace()
			resources[i].Name = u.GetName()
		}
		if resourceErr != nil {
			setInvalidResourceCondition(syncSet, i, resourceErr)
			break
		}
	}

	if resourceErr == nil {
		clearInvalidResourceCondition(syncSet)
	}

	syncSet.GetStatus().ObservedGeneration = syncSet.AsMetaObject().GetGeneration()
	syncSet.GetStatus().Resources = resources

	if err := r.Status().Update(context.Background(), syncSet.AsRuntimeObject()); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update syncset")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func setInvalidResourceCondition(syncSet clustersyncset.CommonSyncSet, resourceIndex int, resourceErr error) {
	syncSet.GetStatus().Conditions = []hivev1.SyncSetCondition{{
		Type:               hivev1.SyncSetInvalidResourceCondition,
		Status:             corev1.ConditionTrue,
		Reason:             "InvalidResource",
		Message:            fmt.Sprintf("Resource %d is invalid: %v", resourceIndex, resourceErr),
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}}
}

func clearInvalidResourceCondition(syncSet clustersyncset.CommonSyncSet) {
	if conds := syncSet.GetStatus().Conditions; len(conds) == 0 {
		return
	}
	syncSet.GetStatus().Conditions = []hivev1.SyncSetCondition{{
		Type:               hivev1.SyncSetInvalidResourceCondition,
		Status:             corev1.ConditionFalse,
		Reason:             "AllResourcesValid",
		Message:            "All resources are valid",
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
	}}
}

package clusterversion

import (
	"context"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	clusterVersionObjectName = "version"
	controllerName           = "clusterversion"
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileClusterVersion{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, controllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusterversion-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterVersion{}

// ReconcileClusterVersion reconciles a ClusterDeployment object
type ReconcileClusterVersion struct {
	client.Client
	scheme *runtime.Scheme

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and syncs the remote ClusterVersion status
// if the remote cluster is available.
func (r *ReconcileClusterVersion) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
		"controller":        controllerName,
	})

	cdLog.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// If the cluster is not installed, do not reconcile.
	if !cd.Spec.Installed {
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)

	// If the cluster is unreachable, do not reconcile.
	if remoteClientBuilder.Unreachable() {
		cdLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	remoteClient, err := remoteClientBuilder.Build()
	if err != nil {
		cdLog.WithError(err).Error("error building remote cluster-api client connection")
		return reconcile.Result{}, err
	}

	clusterVersion := &openshiftapiv1.ClusterVersion{}
	err = remoteClient.Get(context.Background(), types.NamespacedName{Name: clusterVersionObjectName}, clusterVersion)
	if err != nil {
		cdLog.WithError(err).Error("error fetching remote clusterversion object")
		return reconcile.Result{}, err
	}

	err = r.updateClusterVersionStatus(cd, clusterVersion, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	cdLog.Debug("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterVersion) updateClusterVersionStatus(cd *hivev1.ClusterDeployment, clusterVersion *openshiftapiv1.ClusterVersion, cdLog log.FieldLogger) error {
	origCD := cd.DeepCopy()
	cdLog.WithField("clusterversion.status", clusterVersion.Status).Debug("remote cluster version status")
	clusterVersion.Status.DeepCopyInto(&cd.Status.ClusterVersionStatus)

	if reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Debug("status has not changed, nothing to update")
		return nil
	}

	// Update cluster deployment status if changed:
	cdLog.Infof("status has changed, updating cluster deployment")
	err := r.Status().Update(context.TODO(), cd)
	if err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating cluster deployment status")
		return err
	}
	return nil
}

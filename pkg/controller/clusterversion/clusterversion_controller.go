package clusterversion

import (
	"context"
	"fmt"
	"strconv"

	"github.com/blang/semver/v4"
	log "github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	openshiftapiv1 "github.com/openshift/api/config/v1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	clusterVersionObjectName = "version"
	ControllerName           = hivev1.ClusterVersionControllerName
)

// Add creates a new ClusterDeployment Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
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
	r := &ReconcileClusterVersion{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme: mgr.GetScheme(),
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("clusterversion-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}, &handler.TypedEnqueueRequestForObject[*hivev1.ClusterDeployment]{}))
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
func (r *ReconcileClusterVersion) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
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
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
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

	remoteClient, unreachable, requeue := remoteclient.ConnectToRemoteCluster(
		cd,
		r.remoteClusterAPIClientBuilder(cd),
		r.Client,
		cdLog,
	)
	if unreachable {
		return reconcile.Result{Requeue: requeue}, nil
	}

	clusterVersion := &openshiftapiv1.ClusterVersion{}
	err = remoteClient.Get(context.Background(), types.NamespacedName{Name: clusterVersionObjectName}, clusterVersion)
	if err != nil {
		cdLog.WithError(err).Error("error fetching remote clusterversion object")
		return reconcile.Result{}, err
	}

	if err := r.updateClusterVersionLabels(cd, clusterVersion, cdLog); err != nil {
		return reconcile.Result{}, err
	}

	cdLog.Debug("reconcile complete")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterVersion) updateClusterVersionLabels(cd *hivev1.ClusterDeployment, clusterVersion *openshiftapiv1.ClusterVersion, cdLog log.FieldLogger) error {
	changed := false
	cvoVersion := clusterVersion.Status.Desired.Version
	if cd.Labels == nil {
		cd.Labels = map[string]string{}
	}
	if cd.Labels[constants.VersionLabel] != cvoVersion {
		cd.Labels[constants.VersionLabel] = cvoVersion
		changed = true
	}
	if version, err := semver.ParseTolerant(cvoVersion); err == nil {
		major := fmt.Sprintf("%d", version.Major)
		majorMinor := fmt.Sprintf("%s.%d", major, version.Minor)
		majorMinorPatch := fmt.Sprintf("%s.%d", majorMinor, version.Patch)
		changed = changed ||
			cd.Labels[constants.VersionMajorLabel] != major ||
			cd.Labels[constants.VersionMajorMinorLabel] != majorMinor ||
			cd.Labels[constants.VersionMajorMinorPatchLabel] != majorMinorPatch
		cd.Labels[constants.VersionMajorLabel] = major
		cd.Labels[constants.VersionMajorMinorLabel] = majorMinor
		cd.Labels[constants.VersionMajorMinorPatchLabel] = majorMinorPatch
	} else {
		cdLog.WithField("version", cvoVersion).WithError(err).Warn("could not parse the cluster version")
		origLen := len(cd.Labels)
		delete(cd.Labels, constants.VersionMajorLabel)
		delete(cd.Labels, constants.VersionMajorMinorLabel)
		delete(cd.Labels, constants.VersionMajorMinorPatchLabel)
		changed = changed || origLen != len(cd.Labels)
	}

	if !changed {
		cdLog.Debug("labels have not changed, nothing to update")
		return nil
	}

	if err := r.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error update cluster deployment labels")
		return err
	}
	return nil
}

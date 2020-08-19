/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package syncsetinstance

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	hiveresource "github.com/openshift/hive/pkg/resource"
)

const (
	ControllerName           = "syncsetinstance"
	unknownObjectFoundReason = "UnknownObjectFound"
	applySucceededReason     = "ApplySucceeded"
	applyFailedReason        = "ApplyFailed"
	deletionFailedReason     = "DeletionFailed"
	defaultReapplyInterval   = 2 * time.Hour
	reapplyIntervalEnvKey    = "SYNCSET_REAPPLY_INTERVAL"
	secretsResource          = "secrets"
	secretKind               = "Secret"
	secretAPIVersion         = "v1"
	labelApply               = "apply"
	labelCreateOrUpdate      = "createOrUpdate"
	labelCreateOnly          = "createOnly"
	metricResultSuccess      = "success"
	metricResultError        = "error"
)

var (
	applyTempFileMatcher = regexp.MustCompile(`/apply-\S*(\s|$)`)

	metricTimeToApplySyncSet = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "hive_syncset_apply_duration_seconds",
			Help:    "Time to first successfully apply syncset to a cluster",
			Buckets: []float64{5, 10, 30, 60, 300, 600, 1800, 3600},
		},
	)
	metricTimeToApplySelectorSyncSet = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_selectorsyncset_apply_duration_seconds",
			Help:    "Time to first successfully apply selector syncset to a cluster",
			Buckets: []float64{5, 10, 30, 60, 300, 600, 1800, 3600},
		},
		[]string{"name"},
	)

	metricResourcesApplied = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_syncsetinstance_resources_applied_total",
		Help: "Counter incremented each time we sync a resource to a remote cluster, labeled by type of apply and result.",
	},
		[]string{"type", "result"},
	)

	metricTimeToApplySyncSetResource = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_syncsetinstance_apply_duration_seconds",
			Help:    "Time to apply individual resources in a syncset, labeled by type of apply and result.",
			Buckets: []float64{0.5, 1, 3, 5, 10, 20, 30},
		},
		[]string{"type", "result"},
	)
)

func init() {
	metrics.Registry.MustRegister(metricTimeToApplySyncSet)
	metrics.Registry.MustRegister(metricTimeToApplySelectorSyncSet)
	metrics.Registry.MustRegister(metricResourcesApplied)
	metrics.Registry.MustRegister(metricTimeToApplySyncSetResource)
}

// Applier knows how to Apply, Patch and return Info for []byte arrays describing objects and patches.
type Applier interface {
	Apply(obj []byte) (hiveresource.ApplyResult, error)
	Info(obj []byte) (*hiveresource.Info, error)
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (hiveresource.ApplyResult, error)
	CreateOrUpdate(obj []byte) (hiveresource.ApplyResult, error)
	CreateOrUpdateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (hiveresource.ApplyResult, error)
	Create(obj []byte) (hiveresource.ApplyResult, error)
	CreateRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (hiveresource.ApplyResult, error)
}

// Add creates a new SyncSetInstance controller and adds it to the manager with default RBAC. The manager will set fields on the
// controller and start it when the manager starts.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	reapplyInterval := defaultReapplyInterval
	if envReapplyInterval := os.Getenv(reapplyIntervalEnvKey); len(envReapplyInterval) > 0 {
		var err error
		reapplyInterval, err = time.ParseDuration(envReapplyInterval)
		if err != nil {
			log.WithError(err).WithField("reapplyInterval", envReapplyInterval).Errorf("unable to parse %s", reapplyIntervalEnvKey)
			return err
		}
	}
	log.WithField("reapplyInterval", reapplyInterval).Info("Reapply interval set")
	return AddToManager(mgr, NewReconciler(mgr, logger, reapplyInterval))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger log.FieldLogger, reapplyInterval time.Duration) reconcile.Reconciler {
	r := &ReconcileSyncSetInstance{
		Client:          controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName),
		scheme:          mgr.GetScheme(),
		logger:          logger,
		applierBuilder:  applierBuilderFunc,
		reapplyInterval: reapplyInterval,
	}
	r.hash = r.resourceHash
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}
	return r
}

// applierBuilderFunc returns an Applier which implements Info, Apply and Patch
func applierBuilderFunc(restConfig *rest.Config, logger log.FieldLogger) Applier {
	return hiveresource.NewHelperFromRESTConfig(restConfig, logger)
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("syncsetinstance-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to SyncSetInstance
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSetInstance{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployments
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(r.(*ReconcileSyncSetInstance).handleClusterDeployment),
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileSyncSetInstance) handleClusterDeployment(a handler.MapObject) []reconcile.Request {
	cd, ok := a.Object.(*hivev1.ClusterDeployment)
	if !ok {
		return []reconcile.Request{}
	}

	syncSetInstanceList := &hivev1.SyncSetInstanceList{}
	err := r.List(context.TODO(), syncSetInstanceList, client.InNamespace(cd.Namespace))
	if err != nil {
		r.logger.WithError(err).Error("cannot list syncSetInstances for cluster deployment")
		return []reconcile.Request{}
	}

	retval := []reconcile.Request{}
	for _, syncSetInstance := range syncSetInstanceList.Items {
		if metav1.IsControlledBy(&syncSetInstance, cd) {
			retval = append(retval, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      syncSetInstance.Name,
					Namespace: syncSetInstance.Namespace,
				},
			})
		}
	}
	return retval
}

var _ reconcile.Reconciler = &ReconcileSyncSetInstance{}

// ReconcileSyncSetInstance reconciles a ClusterDeployment and the SyncSets associated with it
type ReconcileSyncSetInstance struct {
	client.Client
	scheme *runtime.Scheme

	logger          log.FieldLogger
	applierBuilder  func(*rest.Config, log.FieldLogger) Applier
	hash            func([]byte) string
	reapplyInterval time.Duration

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder
}

// Reconcile applies SyncSet or SelectorSyncSets associated with SyncSetInstances to the owning cluster.
func (r *ReconcileSyncSetInstance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	ssiLog := r.logger.WithField("syncsetinstance", request.NamespacedName)
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(ControllerName).Observe(dur.Seconds())
		ssiLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	ssi := &hivev1.SyncSetInstance{}

	// Fetch the syncsetinstance
	err := r.Get(context.TODO(), request.NamespacedName, ssi)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		r.logger.WithError(err).Error("error looking up syncsetinstance")
		return reconcile.Result{}, err
	}

	if ssi.DeletionTimestamp != nil {
		if !controllerutils.HasFinalizer(ssi, hivev1.FinalizerSyncSetInstance) {
			return reconcile.Result{}, nil
		}
		return r.syncDeletedSyncSetInstance(ssi, ssiLog)
	}

	if !controllerutils.HasFinalizer(ssi, hivev1.FinalizerSyncSetInstance) {
		ssiLog.Debug("adding finalizer")
		return reconcile.Result{}, r.addSyncSetInstanceFinalizer(ssi, ssiLog)
	}

	cd, err := r.getClusterDeployment(ssi, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !controllerutils.ShouldSyncCluster(cd, ssiLog) {
		return reconcile.Result{}, nil
	}

	if !cd.DeletionTimestamp.IsZero() {
		ssiLog.Debug("clusterdeployment is being deleted")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		ssiLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		ssiLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	// If the cluster is unreachable, return from here.
	if unreachable, _ := remoteclient.Unreachable(cd); unreachable {
		ssiLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	ssiLog.Info("reconciling syncsetinstance")
	spec, deleted, err := r.getSyncSetCommonSpec(ssi, ssiLog)
	if (!deleted && spec == nil) || err != nil {
		return reconcile.Result{}, err
	}
	if deleted {
		ssiLog.Info("source has been deleted, deleting syncsetinstance")
		err = r.Delete(context.TODO(), ssi)
		if err != nil {
			ssiLog.WithError(err).Log(controllerutils.LogLevel(err), "failed to delete syncsetinstance")
		}
		return reconcile.Result{}, err
	}

	// Prevent users with permissions to create a SyncSet from referencing Secrets in another namespace
	// and copying them into their clusters. This should be caught in admission, but just incase we have
	// a check here to refuse to reconcile. Error is logged but not returned as there's no point re-reconciling
	// this unless the SyncSet is edited. This check is not relevant for SelectorSyncSets which have no namespace.
	if r.hasSourceSecretsInAnotherNamespace(ssi, spec) {
		ssiLog.Warn("syncsetinstance has a source secret in another namespace")
		return reconcile.Result{}, nil
	}

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)
	dynamicClient, unreachable, requeue := remoteclient.ConnectToRemoteClusterWithDynamicClient(
		cd,
		remoteClientBuilder,
		r.Client,
		ssiLog,
	)
	if unreachable {
		return reconcile.Result{Requeue: requeue}, nil
	}
	restConfig, err := remoteClientBuilder.RESTConfig()
	if err != nil {
		ssiLog.WithError(err).Error("unable to get REST config")
		return reconcile.Result{}, err
	}

	ssiLog.Debug("applying sync set")
	original := ssi.DeepCopy()

	applier := r.applierBuilder(restConfig, ssiLog)
	applyErr := r.applySyncSet(ssi, spec, dynamicClient, applier, ssiLog)
	ssi.Status.Applied = applyErr == nil

	var applyTime float64
	if ssi.Status.Applied && ssi.Status.FirstSuccessTimestamp == nil {
		now := metav1.Now()
		ssi.Status.FirstSuccessTimestamp = &now
		if !original.Status.Applied {
			startTime := ssi.CreationTimestamp.Time
			if cd.Status.InstalledTimestamp != nil && startTime.Before(cd.Status.InstalledTimestamp.Time) {
				startTime = cd.Status.InstalledTimestamp.Time
			}
			if cond := controllerutils.FindClusterDeploymentCondition(cd.Status.Conditions,
				hivev1.UnreachableCondition); cond != nil && startTime.Before(cond.LastTransitionTime.Time) {
				startTime = cond.LastTransitionTime.Time
			}
			applyTime = float64(ssi.Status.FirstSuccessTimestamp.Sub(startTime).Seconds())
		}
	}

	err = r.updateSyncSetInstanceStatus(ssi, original, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	ssiLog.Info("done reconciling syncsetinstance")
	if applyErr != nil {
		ssiLog.WithError(applyErr).Warn("failed to apply, requeueing")
		// Set requeue to true instead of returning the applyErr to requeue the syncsetinstance.
		return reconcile.Result{Requeue: true}, nil
	}

	if applyTime != 0 {
		if sss := ssi.Spec.SelectorSyncSetRef; sss != nil {
			metricTimeToApplySelectorSyncSet.WithLabelValues(sss.Name).Observe(applyTime)
		} else {
			metricTimeToApplySyncSet.Observe(applyTime)
		}
	}

	reapplyDuration := r.ssiReapplyDuration(ssi)
	return reconcile.Result{RequeueAfter: reapplyDuration}, nil
}

func (r *ReconcileSyncSetInstance) hasSourceSecretsInAnotherNamespace(ssi *hivev1.SyncSetInstance, spec *hivev1.SyncSetCommonSpec) bool {
	// This check is not relevant for SelectorSyncSets
	if ssi.Spec.SyncSetRef == nil {
		return false
	}
	for _, secret := range spec.Secrets {
		if secret.SourceRef.Namespace != ssi.Namespace && secret.SourceRef.Namespace != "" {
			return true
		}
	}
	return false
}

// ssiReapplyDuration returns the shortest time.Duration to meet reapplyInterval from successfully applied
// resources, patches and secrets in the SyncSetInstance status.
func (r *ReconcileSyncSetInstance) ssiReapplyDuration(ssi *hivev1.SyncSetInstance) time.Duration {
	var nextApplyTime time.Time
	for _, statuses := range [][]hivev1.SyncStatus{ssi.Status.Resources, ssi.Status.Patches, ssi.Status.Secrets} {
		for _, status := range statuses {
			reapplyTime := r.reapplyTime(status)
			if nextApplyTime.IsZero() || reapplyTime.Before(nextApplyTime) {
				nextApplyTime = reapplyTime
			}
		}
	}
	wait := time.Until(nextApplyTime)
	if wait < 0 {
		return 0
	}
	return wait + time.Duration(0.1*rand.Float64()*r.reapplyInterval.Seconds())*time.Second
}

func (r *ReconcileSyncSetInstance) getClusterDeployment(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{}
	cdName := types.NamespacedName{Namespace: ssi.Namespace, Name: ssi.Spec.ClusterDeploymentRef.Name}
	ssiLog = ssiLog.WithField("clusterdeployment", cdName)
	err := r.Get(context.TODO(), cdName, cd)
	if err != nil {
		ssiLog.WithError(err).Error("error looking up clusterdeployment")
		return nil, err
	}
	return cd, nil
}

func (r *ReconcileSyncSetInstance) addSyncSetInstanceFinalizer(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) error {
	ssiLog.Debug("adding finalizer")
	controllerutils.AddFinalizer(ssi, hivev1.FinalizerSyncSetInstance)
	err := r.Update(context.TODO(), ssi)
	if err != nil {
		ssiLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot add finalizer")
	}
	return err
}

func (r *ReconcileSyncSetInstance) removeSyncSetInstanceFinalizer(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) error {
	ssiLog.Debug("removing finalizer")
	controllerutils.DeleteFinalizer(ssi, hivev1.FinalizerSyncSetInstance)
	err := r.Update(context.TODO(), ssi)
	if err != nil {
		ssiLog.WithError(err).Log(controllerutils.LogLevel(err), "cannot remove finalizer")
	}
	return err
}

func (r *ReconcileSyncSetInstance) syncDeletedSyncSetInstance(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) (reconcile.Result, error) {
	if ssi.Spec.ResourceApplyMode != hivev1.SyncResourceApplyMode {
		ssiLog.Debug("syncset is deleted and there is nothing to clean up, removing finalizer")
		return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
	}
	cd, err := r.getClusterDeployment(ssi, ssiLog)
	if errors.IsNotFound(err) {
		// clusterdeployment has been deleted, should just remove the finalizer
		return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
	} else if err != nil {
		// unknown error, try again
		return reconcile.Result{}, err
	}

	if !cd.DeletionTimestamp.IsZero() {
		ssiLog.Debug("clusterdeployment is being deleted")
		return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
	}

	dynamicClient, unreachable, requeue := remoteclient.ConnectToRemoteClusterWithDynamicClient(
		cd,
		r.remoteClusterAPIClientBuilder(cd),
		r.Client,
		ssiLog,
	)
	if unreachable {
		return reconcile.Result{Requeue: requeue}, nil
	}

	ssiLog.Info("deleting syncset resources on target cluster")
	err = r.deleteSyncSetResources(ssi, dynamicClient, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.deleteSyncSetSecrets(ssi, dynamicClient, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
}

func (r *ReconcileSyncSetInstance) applySyncSet(ssi *hivev1.SyncSetInstance, spec *hivev1.SyncSetCommonSpec, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	defer func() {
		// Temporary fix for status hot loop: do not update ssi.Status.{Patches,Resources,Secrets} with empty slice.
		if len(ssi.Status.Resources) == 0 {
			ssi.Status.Resources = nil
		}
		if len(ssi.Status.Patches) == 0 {
			ssi.Status.Patches = nil
		}
		if len(ssi.Status.Secrets) == 0 {
			ssi.Status.Secrets = nil
		}
	}()

	if err := r.applySyncSetResources(ssi, spec.Resources, spec.ApplyBehavior, dynamicClient, h, ssiLog); err != nil {
		return err
	}
	if err := r.applySyncSetPatches(ssi, spec.Patches, h, ssiLog); err != nil {
		return err
	}
	return r.applySyncSetSecretMappings(ssi, spec.Secrets, spec.ApplyBehavior, dynamicClient, h, ssiLog)
}

func (r *ReconcileSyncSetInstance) deleteSyncSetResources(ssi *hivev1.SyncSetInstance, dynamicClient dynamic.Interface, ssiLog log.FieldLogger) error {
	var lastError error
	for index, resourceStatus := range ssi.Status.Resources {
		itemLog := ssiLog.WithField("resource", fmt.Sprintf("%s/%s", resourceStatus.Namespace, resourceStatus.Name)).
			WithField("apiversion", resourceStatus.APIVersion).
			WithField("kind", resourceStatus.Kind)
		unknownObjectSyncCondition := controllerutils.FindSyncCondition(resourceStatus.Conditions, hivev1.UnknownObjectSyncCondition)
		if unknownObjectSyncCondition != nil {
			itemLog.Info("UnknownObject condition is set, skipping deletion")
			continue
		}
		gv, err := schema.ParseGroupVersion(resourceStatus.APIVersion)
		if err != nil {
			itemLog.WithError(err).Warn("cannot parse resource apiVersion, skipping deletion")
			continue
		}
		gvr := gv.WithResource(resourceStatus.Resource)
		itemLog.Debug("deleting resource")
		err = dynamicClient.Resource(gvr).Namespace(resourceStatus.Namespace).Delete(context.Background(), resourceStatus.Name, metav1.DeleteOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				itemLog.Debug("resource not found, nothing to do")
			case errors.IsForbidden(err):
				itemLog.WithError(err).Warn("forbidden resource deletion, skipping")
			default:
				lastError = err
				itemLog.WithError(err).Warn("error deleting resource")
				ssi.Status.Resources[index].Conditions = r.setDeletionFailedSyncCondition(ssi.Status.Resources[index].Conditions, fmt.Errorf("failed to delete resource: %v", err))
			}
		}
	}
	return lastError
}

func (r *ReconcileSyncSetInstance) deleteSyncSetSecrets(ssi *hivev1.SyncSetInstance, dynamicClient dynamic.Interface, ssiLog log.FieldLogger) error {
	var lastError error
	for index, secretStatus := range ssi.Status.Secrets {
		secretLog := ssiLog.WithField("secret", fmt.Sprintf("%s/%s", secretStatus.Namespace, secretStatus.Name)).
			WithField("apiVersion", secretStatus.APIVersion).
			WithField("kind", secretStatus.Kind)
		gv, err := schema.ParseGroupVersion(secretStatus.APIVersion)
		if err != nil {
			secretLog.WithError(err).Warn("cannot parse secret apiVersion, skipping deletion")
			continue
		}
		gvr := gv.WithResource(secretStatus.Resource)
		secretLog.Debug("deleting secret")
		err = dynamicClient.Resource(gvr).Namespace(secretStatus.Namespace).Delete(context.Background(), secretStatus.Name, metav1.DeleteOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				secretLog.Debug("secret not found, nothing to do")
			case errors.IsForbidden(err):
				secretLog.WithError(err).Warn("forbidden secret deletion, skipping")
			default:
				lastError = err
				secretLog.WithError(err).Warn("error deleting secret")
				ssi.Status.Secrets[index].Conditions = r.setDeletionFailedSyncCondition(ssi.Status.Secrets[index].Conditions, fmt.Errorf("failed to delete secret: %v", err))
			}
		}
	}
	return lastError
}

// getSyncSetCommonSpec returns the common spec of the associated syncset or selectorsyncset. It returns a boolean indicating
// whether the source object (syncset or selectorsyncset) has been deleted or is in the process of being deleted.
func (r *ReconcileSyncSetInstance) getSyncSetCommonSpec(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) (*hivev1.SyncSetCommonSpec, bool, error) {
	if ssi.Spec.SyncSetRef != nil {
		syncSet := &hivev1.SyncSet{}
		syncSetName := types.NamespacedName{Namespace: ssi.Namespace, Name: ssi.Spec.SyncSetRef.Name}
		err := r.Get(context.TODO(), syncSetName, syncSet)
		if errors.IsNotFound(err) || !syncSet.DeletionTimestamp.IsZero() {
			ssiLog.WithError(err).WithField("syncset", syncSetName).Warning("syncset is being deleted")
			return nil, true, nil
		}
		if err != nil {
			ssiLog.WithError(err).WithField("syncset", syncSetName).Error("cannot get associated syncset")
			return nil, false, err
		}
		return &syncSet.Spec.SyncSetCommonSpec, false, nil
	} else if ssi.Spec.SelectorSyncSetRef != nil {
		selectorSyncSet := &hivev1.SelectorSyncSet{}
		selectorSyncSetName := types.NamespacedName{Name: ssi.Spec.SelectorSyncSetRef.Name}
		err := r.Get(context.TODO(), selectorSyncSetName, selectorSyncSet)
		if errors.IsNotFound(err) || !selectorSyncSet.DeletionTimestamp.IsZero() {
			ssiLog.WithField("selectorsyncset", selectorSyncSetName).Warning("selectorsyncset is being deleted")
			return nil, true, nil
		}
		if err != nil {
			ssiLog.WithField("selectorsyncset", selectorSyncSetName).Error("cannot get associated selectorsyncset")
			return nil, false, err
		}
		return &selectorSyncSet.Spec.SyncSetCommonSpec, false, nil
	}
	ssiLog.Error("invalid syncsetinstance, no reference found to syncset or selectorsyncset")
	return nil, false, nil
}

// findSyncStatus returns a SyncStatus matching the provided status from a list of SyncStatus
func findSyncStatus(status hivev1.SyncStatus, statusList []hivev1.SyncStatus) *hivev1.SyncStatus {
	for _, ss := range statusList {
		if status.Name == ss.Name &&
			status.Namespace == ss.Namespace &&
			status.APIVersion == ss.APIVersion &&
			status.Kind == ss.Kind {
			return &ss
		}
	}
	return nil
}

// needToReapply determines if the provided status indicates that the resource, secret or patch needs to be re-applied
func (r *ReconcileSyncSetInstance) needToReapply(newStatus, existingStatus hivev1.SyncStatus, ssiLog log.FieldLogger) bool {
	// Re-apply if hash has changed
	if existingStatus.Hash != newStatus.Hash {
		ssiLog.Debug("hash has changed, will re-apply")
		return true
	}
	// Re-apply if failure occurred
	if failureCondition := controllerutils.FindSyncCondition(existingStatus.Conditions, hivev1.ApplyFailureSyncCondition); failureCondition != nil {
		if failureCondition.Status == corev1.ConditionTrue {
			ssiLog.Debug("failed last time, will re-apply")
			return true
		}
	}
	if r.reapplyTime(existingStatus).Before(time.Now()) {
		ssiLog.Debug("re-apply duration has elapsed, will re-apply")
		return true
	}
	return false
}

func (r *ReconcileSyncSetInstance) reapplyTime(status hivev1.SyncStatus) time.Time {
	applySuccessCondition := controllerutils.FindSyncCondition(status.Conditions, hivev1.ApplySuccessSyncCondition)
	if applySuccessCondition == nil {
		return time.Now()
	}
	return applySuccessCondition.LastProbeTime.Add(r.reapplyInterval)
}

// applySyncSetResources evaluates resource objects from RawExtension and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetResources(ssi *hivev1.SyncSetInstance, resources []runtime.RawExtension, applyBehavior hivev1.SyncSetApplyBehavior, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	ssiLog = ssiLog.WithField("applyTerm", "resource")

	syncStatusList := []hivev1.SyncStatus{}

	applyFn := h.Apply
	applyFnMetricsLabel := labelApply
	switch applyBehavior {
	case hivev1.CreateOrUpdateSyncSetApplyBehavior:
		applyFn = h.CreateOrUpdate
		applyFnMetricsLabel = labelCreateOrUpdate
	case hivev1.CreateOnlySyncSetApplyBehavior:
		applyFn = h.Create
		applyFnMetricsLabel = labelCreateOnly
	}
	// determine if we can gather info for the resource and apply it
	var applyErr error
	var discoveryErr error
	info := &hiveresource.Info{}
	for _, resource := range resources {
		resourceSyncStatus := hivev1.SyncStatus{}

		ssiLog := ssiLog.WithField("syncNamespace", resourceSyncStatus.Namespace).
			WithField("syncName", resourceSyncStatus.Name).
			WithField("syncAPIVersion", resourceSyncStatus.APIVersion).
			WithField("syncKind", resourceSyncStatus.Kind)

		info, discoveryErr = h.Info(resource.Raw)
		// If an error occurred, stop processing right here
		if discoveryErr != nil {
			resourceSyncStatus.Conditions = r.setUnknownObjectSyncCondition(resourceSyncStatus.Conditions, discoveryErr)
			syncStatusList = append(syncStatusList, resourceSyncStatus)
			ssiLog.WithError(discoveryErr).Warn("unable to parse resource")
			break
		}

		resourceSyncStatus.APIVersion = info.APIVersion
		resourceSyncStatus.Kind = info.Kind
		resourceSyncStatus.Name = info.Name
		resourceSyncStatus.Namespace = info.Namespace
		resourceSyncStatus.Hash = r.hash(resource.Raw)
		resourceSyncStatus.Resource = info.Resource
		resourceSyncStatus.Conditions = r.clearUnknownObjectSyncCondition(resourceSyncStatus.Conditions)

		if rss := findSyncStatus(resourceSyncStatus, ssi.Status.Resources); rss == nil || r.needToReapply(resourceSyncStatus, *rss, ssiLog) {
			// Apply resource
			ssiLog.Debug("applying resource")
			applyErr = applyResource(ssi, info.Object, ssiLog, applyFnMetricsLabel, applyFn)

			var resourceSyncConditions []hivev1.SyncCondition
			if rss != nil {
				resourceSyncConditions = rss.Conditions
			}
			resourceSyncStatus.Conditions = r.setApplySyncConditions(resourceSyncConditions, applyErr)

		} else {
			// Do not apply resource
			ssiLog.Debug("resource has not changed, will not apply")
			resourceSyncStatus.Conditions = rss.Conditions
		}

		syncStatusList = append(syncStatusList, resourceSyncStatus)

		// If an error applying occurred, stop processing right here
		if applyErr != nil {
			break
		}
	}

	ssi.Status.Resources = r.reconcileDeleted(ssi.Spec.ResourceApplyMode, dynamicClient, ssi.Status.Resources, syncStatusList, applyErr, ssiLog)
	// Return applyErr/discoveryErr for the controller to trigger retries and go into exponential backoff
	// if the problem does not resolve itself.
	if discoveryErr != nil {
		return discoveryErr
	}
	if applyErr != nil {
		return applyErr
	}

	return nil
}

// addHiveLabels injects labels to help end-users and support see that a resource is managed by hive,
// and which [Selector]SyncSet it originated from.
func addHiveLabels(ssi *hivev1.SyncSetInstance, obj metav1.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 2)
	}
	// Resource is managed by hive:
	labels[constants.HiveManagedLabel] = "true"
	// Which [Selector]SyncSet it originated from:
	if ssi.Spec.SelectorSyncSetRef != nil {
		labels[constants.SelectorSyncSetNameLabel] = ssi.Spec.SelectorSyncSetRef.Name
	} else if ssi.Spec.SyncSetRef != nil {
		labels[constants.SyncSetNameLabel] = ssi.Spec.SyncSetRef.Name
	}
	// TODO(efried): else panic? This is a developer error (forgot to add a condition for new ssi source type).
	// For now, this is super safe; worst case we just omit the label.

	obj.SetLabels(labels)
}

func applyResource(ssi *hivev1.SyncSetInstance, u *unstructured.Unstructured, logger log.FieldLogger, applyFnMetricLabel string, applyFn func(obj []byte) (hiveresource.ApplyResult, error)) error {

	startTime := time.Now()

	addHiveLabels(ssi, u)

	bytes, err := json.Marshal(u)
	if err != nil {
		logger.WithError(err).Error("error marshalling unstructured object to json bytes")
		return err
	}

	applyResult, err := applyFn(bytes)
	// Record the amount of time we took to apply this specific resource. When combined with the metric for duration of
	// our kube client requests, we can get an idea how much time we're spending cpu bound vs network bound.
	applyTime := metav1.Now().Sub(startTime).Seconds()
	if err != nil {
		logger.WithError(err).Warn("error applying resource")
		metricResourcesApplied.WithLabelValues(applyFnMetricLabel, metricResultError).Inc()
		metricTimeToApplySyncSetResource.WithLabelValues(applyFnMetricLabel, metricResultError).Observe(applyTime)
	} else {
		logger.WithField("applyResult", applyResult).Debug("resource applied")
		metricResourcesApplied.WithLabelValues(applyFnMetricLabel, metricResultSuccess).Inc()
		metricTimeToApplySyncSetResource.WithLabelValues(applyFnMetricLabel, metricResultSuccess).Observe(applyTime)
	}
	return err
}

func (r *ReconcileSyncSetInstance) reconcileDeleted(applyMode hivev1.SyncSetResourceApplyMode, dynamicClient dynamic.Interface, existingStatusList, newStatusList []hivev1.SyncStatus, err error, ssiLog log.FieldLogger) []hivev1.SyncStatus {
	ssiLog.WithField("existing", len(existingStatusList)).WithField("new", len(newStatusList)).Debug("reconciling syncset")
	if applyMode == "" || applyMode == hivev1.UpsertResourceApplyMode {
		ssiLog.Debug("resourceApplyMode is Upsert, remote will not be deleted")
		return newStatusList
	}

	deletedStatusList := []hivev1.SyncStatus{}
	for _, existingStatus := range existingStatusList {
		if ss := findSyncStatus(existingStatus, newStatusList); ss == nil {
			ssiLog.WithField("syncNamespace", existingStatus.Namespace).
				WithField("syncName", existingStatus.Name).
				WithField("syncAPIVersion", existingStatus.APIVersion).
				WithField("syncKind", existingStatus.Kind).
				Debug("not found in updated status, will queue up for deletion")
			deletedStatusList = append(deletedStatusList, existingStatus)
		}
	}

	// If an error occurred applying resources, do not delete yet
	if err != nil {
		ssiLog.Debug("an error occurred applying, will preserve all syncset status items")
		return append(newStatusList, deletedStatusList...)
	}

	for _, deletedStatus := range deletedStatusList {
		itemLog := ssiLog.WithField("syncNamespace", deletedStatus.Namespace).
			WithField("syncName", deletedStatus.Name).
			WithField("syncAPIVersion", deletedStatus.APIVersion).
			WithField("syncKind", deletedStatus.Kind)
		gv, err := schema.ParseGroupVersion(deletedStatus.APIVersion)
		if err != nil {
			itemLog.WithError(err).Warn("unable to delete, cannot parse group version")
			deletedStatus.Conditions = r.setDeletionFailedSyncCondition(deletedStatus.Conditions, err)
			newStatusList = append(newStatusList, deletedStatus)
			continue
		}
		gvr := gv.WithResource(deletedStatus.Resource)
		itemLog.Debug("deleting")
		err = dynamicClient.Resource(gvr).Namespace(deletedStatus.Namespace).Delete(context.Background(), deletedStatus.Name, metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				itemLog.WithError(err).Warn("error deleting")
				deletedStatus.Conditions = r.setDeletionFailedSyncCondition(deletedStatus.Conditions, err)
				newStatusList = append(newStatusList, deletedStatus)
			} else {
				itemLog.Debug("not found, nothing to do")
			}
		}
	}

	return newStatusList
}

// applySyncSetPatches applies patches to cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetPatches(ssi *hivev1.SyncSetInstance, ssPatches []hivev1.SyncObjectPatch, h Applier, ssiLog log.FieldLogger) error {
	ssiLog = ssiLog.WithField("applyTerm", "patch")

	for _, ssPatch := range ssPatches {

		b, err := json.Marshal(ssPatch)
		if err != nil {
			ssiLog.WithError(err).Warn("cannot serialize syncset patch")
			return err
		}
		patchSyncStatus := hivev1.SyncStatus{
			APIVersion: ssPatch.APIVersion,
			Kind:       ssPatch.Kind,
			Name:       ssPatch.Name,
			Namespace:  ssPatch.Namespace,
			Hash:       r.hash(b),
		}

		if pss := findSyncStatus(patchSyncStatus, ssi.Status.Patches); pss == nil || r.needToReapply(patchSyncStatus, *pss, ssiLog) {
			ssiLog := ssiLog.WithField("syncNamespace", ssPatch.Namespace).
				WithField("syncName", ssPatch.Name).
				WithField("syncAPIVersion", ssPatch.APIVersion).
				WithField("syncKind", ssPatch.Kind)

			// Apply patch
			ssiLog.Debug("applying patch")
			namespacedName := types.NamespacedName{
				Name:      ssPatch.Name,
				Namespace: ssPatch.Namespace,
			}
			err := h.Patch(namespacedName, ssPatch.Kind, ssPatch.APIVersion, []byte(ssPatch.Patch), ssPatch.PatchType)

			var patchSyncConditions []hivev1.SyncCondition
			if pss != nil {
				patchSyncConditions = pss.Conditions
			}
			patchSyncStatus.Conditions = r.setApplySyncConditions(patchSyncConditions, err)

			ssi.Status.Patches = appendOrUpdateSyncStatus(ssi.Status.Patches, patchSyncStatus)
			if err != nil {
				ssiLog.WithError(err).Warn("error applying patch")
				return err
			}
		} else {
			// Do not apply patch
			ssiLog.Debug("patch has not changed, will not apply")
			patchSyncStatus.Conditions = pss.Conditions
		}
	}
	return nil
}

// applySyncSetSecretMappings evaluates secret mappings and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetSecretMappings(ssi *hivev1.SyncSetInstance, secretMappings []hivev1.SecretMapping, applyBehavior hivev1.SyncSetApplyBehavior, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	ssiLog = ssiLog.WithField("applyTerm", "secret")

	syncStatusList := []hivev1.SyncStatus{}

	applyFn := h.ApplyRuntimeObject
	switch applyBehavior {
	case hivev1.CreateOrUpdateSyncSetApplyBehavior:
		applyFn = h.CreateOrUpdateRuntimeObject
	case hivev1.CreateOnlySyncSetApplyBehavior:
		applyFn = h.CreateRuntimeObject
	}

	var applyErr error
	for _, secretMapping := range secretMappings {
		secretSyncStatus := hivev1.SyncStatus{
			APIVersion: secretAPIVersion,
			Kind:       secretKind,
			Name:       secretMapping.TargetRef.Name,
			Namespace:  secretMapping.TargetRef.Namespace,
			Resource:   secretsResource,
		}

		ssiLog := ssiLog.WithField("syncNamespace", secretSyncStatus.Namespace).
			WithField("syncName", secretSyncStatus.Name).
			WithField("syncAPIVersion", secretSyncStatus.APIVersion).
			WithField("syncKind", secretSyncStatus.Kind)

		rss := findSyncStatus(secretSyncStatus, ssi.Status.Secrets)
		var secretSyncConditions []hivev1.SyncCondition
		if rss != nil {
			secretSyncConditions = rss.Conditions
		}

		secret := &corev1.Secret{}
		applyErr = r.Get(context.Background(), types.NamespacedName{Name: secretMapping.SourceRef.Name, Namespace: secretMapping.SourceRef.Namespace}, secret)
		if applyErr != nil {
			logLevel := log.ErrorLevel
			if errors.IsNotFound(applyErr) {
				logLevel = log.InfoLevel
			}
			ssiLog.WithError(applyErr).Log(logLevel, "cannot read secret")
			secretSyncStatus.Conditions = r.setApplySyncConditions(secretSyncConditions, applyErr)
			syncStatusList = append(syncStatusList, secretSyncStatus)
			break
		}

		secret.Name = secretMapping.TargetRef.Name
		secret.Namespace = secretMapping.TargetRef.Namespace
		// These pieces of metadata need to be set to nil values to perform an update from the original secret
		secret.Generation = 0
		secret.ResourceVersion = ""
		secret.UID = ""
		secret.OwnerReferences = nil

		addHiveLabels(ssi, secret)

		var hash string
		hash, applyErr = controllerutils.GetChecksumOfObject(secret)
		if applyErr != nil {
			ssiLog.WithError(applyErr).Warn("unable to compute secret hash")
			secretSyncStatus.Conditions = r.setApplySyncConditions(secretSyncConditions, applyErr)
			syncStatusList = append(syncStatusList, secretSyncStatus)
			break
		}
		secretSyncStatus.Hash = hash

		if rss == nil || r.needToReapply(secretSyncStatus, *rss, ssiLog) {
			// Apply secret
			ssiLog.Debug("applying secret")
			var result hiveresource.ApplyResult
			result, applyErr = applyFn(secret, scheme.Scheme)

			var secretSyncConditions []hivev1.SyncCondition
			if rss != nil {
				secretSyncConditions = rss.Conditions
			}
			secretSyncStatus.Conditions = r.setApplySyncConditions(secretSyncConditions, applyErr)

			if applyErr != nil {
				ssiLog.WithError(applyErr).Warn("error applying secret")
			} else {
				ssiLog.WithField("applyResult", result).Debug("resource applied")
			}
		} else {
			// Do not apply secret
			ssiLog.Debug("resource has not changed, will not apply")
			secretSyncStatus.Conditions = rss.Conditions
		}

		syncStatusList = append(syncStatusList, secretSyncStatus)

		// If an error applying occurred, stop processing right here
		if applyErr != nil {
			break
		}
	}

	ssi.Status.Secrets = r.reconcileDeleted(ssi.Spec.ResourceApplyMode, dynamicClient, ssi.Status.Secrets, syncStatusList, applyErr, ssiLog)

	// Return applyErr for the controller to trigger retries nd go into exponential backoff
	// if the problem does not resolve itself.
	if applyErr != nil {
		return applyErr
	}

	return nil
}

func appendOrUpdateSyncStatus(statusList []hivev1.SyncStatus, syncStatus hivev1.SyncStatus) []hivev1.SyncStatus {
	for i, ss := range statusList {
		if ss.Name == syncStatus.Name && ss.Namespace == syncStatus.Namespace && ss.Kind == syncStatus.Kind {
			statusList[i] = syncStatus
			return statusList
		}
	}
	return append(statusList, syncStatus)
}

func (r *ReconcileSyncSetInstance) updateSyncSetInstanceStatus(ssi *hivev1.SyncSetInstance, original *hivev1.SyncSetInstance, ssiLog log.FieldLogger) error {
	// Update syncsetinstance status if changed:
	if !reflect.DeepEqual(ssi.Status, original.Status) {
		ssiLog.Infof("syncset instance status has changed, updating")
		if err := r.Status().Update(context.TODO(), ssi); err != nil {
			ssiLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating syncsetinstance status")
			return err
		}
	}
	return nil
}

func (r *ReconcileSyncSetInstance) setUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition, err error) []hivev1.SyncCondition {
	return controllerutils.SetSyncCondition(
		syncSetConditions,
		hivev1.UnknownObjectSyncCondition,
		corev1.ConditionTrue,
		unknownObjectFoundReason,
		fmt.Sprintf("Unable to gather Info for SyncSet resource: %v", err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
}

func (r *ReconcileSyncSetInstance) clearUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition) []hivev1.SyncCondition {
	return controllerutils.SetSyncCondition(
		syncSetConditions,
		hivev1.UnknownObjectSyncCondition,
		corev1.ConditionFalse,
		unknownObjectFoundReason,
		fmt.Sprintf("Info available for SyncSet resource"),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
}

// filterApplyError removes the beginning of applyErrs which match the applyTempFileMatcher regex,
// removing the temporary filename generated by kubectl apply.
// The message we set in the apply failed status condition needs to remain static so that we don't
// update with a new message every reconcile as condition updates rely on
// controllerutils.UpdateConditionIfReasonOrMessageChange.
// Ex:
//     applyErr="error when creating \"/tmp/apply-475927931\": namespaces \"openshift-am-config\" not found"
// Returns:
//     "namespaces \"openshift-am-config\" not found"
func filterApplyError(applyErr string) string {
	loc := applyTempFileMatcher.FindStringIndex(applyErr)
	if loc == nil {
		return applyErr
	}
	return applyErr[loc[1]:]
}

func (r *ReconcileSyncSetInstance) setApplySyncConditions(resourceSyncConditions []hivev1.SyncCondition, err error) []hivev1.SyncCondition {
	var reason, message string
	var successStatus, failureStatus corev1.ConditionStatus
	var updateCondition controllerutils.UpdateConditionCheck
	if err == nil {
		reason = applySucceededReason
		message = "Apply successful"
		successStatus = corev1.ConditionTrue
		failureStatus = corev1.ConditionFalse
		updateCondition = controllerutils.UpdateConditionAlways
	} else {
		reason = applyFailedReason
		message = fmt.Sprintf("Apply failed: %s", filterApplyError(err.Error()))
		successStatus = corev1.ConditionFalse
		failureStatus = corev1.ConditionTrue
		updateCondition = controllerutils.UpdateConditionIfReasonOrMessageChange
	}
	resourceSyncConditions = controllerutils.SetSyncCondition(
		resourceSyncConditions,
		hivev1.ApplySuccessSyncCondition,
		successStatus,
		reason,
		message,
		updateCondition)
	resourceSyncConditions = controllerutils.SetSyncCondition(
		resourceSyncConditions,
		hivev1.ApplyFailureSyncCondition,
		failureStatus,
		reason,
		message,
		updateCondition)

	// If we are reporting that apply succeeded or failed, it means we no longer
	// want to delete this resource. Set that failure condition to false in case
	// it was previously set to true.
	resourceSyncConditions = controllerutils.SetSyncCondition(
		resourceSyncConditions,
		hivev1.DeletionFailedSyncCondition,
		corev1.ConditionFalse,
		reason,
		message,
		updateCondition)
	return resourceSyncConditions
}

func (r *ReconcileSyncSetInstance) setDeletionFailedSyncCondition(resourceSyncConditions []hivev1.SyncCondition, err error) []hivev1.SyncCondition {
	if err == nil {
		return resourceSyncConditions
	}
	return controllerutils.SetSyncCondition(
		resourceSyncConditions,
		hivev1.DeletionFailedSyncCondition,
		corev1.ConditionTrue,
		deletionFailedReason,
		fmt.Sprintf("Failed to delete resource: %v", err),
		controllerutils.UpdateConditionIfReasonOrMessageChange)
}

func (r *ReconcileSyncSetInstance) resourceHash(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

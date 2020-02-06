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
	"os"
	"reflect"
	"regexp"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	controllerName           = "syncsetinstance"
	unknownObjectFoundReason = "UnknownObjectFound"
	applySucceededReason     = "ApplySucceeded"
	applyFailedReason        = "ApplyFailed"
	deletionFailedReason     = "DeletionFailed"
	defaultReapplyInterval   = 2 * time.Hour
	reapplyIntervalEnvKey    = "SYNCSET_REAPPLY_INTERVAL"
	secretsResource          = "secrets"
	secretKind               = "Secret"
	secretAPIVersion         = "v1"
)

var (
	applyTempFileMatcher = regexp.MustCompile(`/apply-\S*(\s|$)`)
)

// Applier knows how to Apply, Patch and return Info for []byte arrays describing objects and patches.
type Applier interface {
	Apply(obj []byte) (hiveresource.ApplyResult, error)
	Info(obj []byte) (*hiveresource.Info, error)
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (hiveresource.ApplyResult, error)
}

// Add creates a new SyncSetInstance controller and adds it to the manager with default RBAC. The manager will set fields on the
// controller and start it when the manager starts.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", controllerName)
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
		Client:          controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:          mgr.GetScheme(),
		logger:          logger,
		applierBuilder:  applierBuilderFunc,
		reapplyInterval: reapplyInterval,
	}
	r.hash = r.resourceHash
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, controllerName)
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
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
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

	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		log.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
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

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)

	// If the cluster is unreachable, return from here.
	if remoteClientBuilder.Unreachable() {
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
	dynamicClient, err := remoteClientBuilder.BuildDynamic()
	if err != nil {
		ssiLog.WithError(err).Error("unable to build dynamic client")
		return reconcile.Result{}, err
	}
	restConfig, err := remoteClientBuilder.RESTConfig()
	if err != nil {
		ssiLog.WithError(err).Error("unable to get REST config")
	}
	ssiLog.Debug("applying sync set")
	original := ssi.DeepCopy()

	applier := r.applierBuilder(restConfig, ssiLog)
	applyErr := r.applySyncSet(ssi, spec, dynamicClient, applier, ssiLog)
	ssi.Status.Applied = applyErr == nil

	err = r.updateSyncSetInstanceStatus(ssi, original, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	ssiLog.Info("done reconciling syncsetinstance")
	if applyErr != nil {
		return reconcile.Result{}, applyErr
	}

	reapplyDuration := ssiReapplyDuration(ssi, r.reapplyInterval)
	return reconcile.Result{RequeueAfter: reapplyDuration}, nil
}

// ssiReapplyDuration returns the shortest time.Duration to meet reapplyInterval from successfully applied
// resources, patches and secrets in the SyncSetInstance status.
func ssiReapplyDuration(ssi *hivev1.SyncSetInstance, reapplyInterval time.Duration) time.Duration {
	timeSinceOldestApply := time.Duration(0)
	for _, statuses := range [][]hivev1.SyncStatus{ssi.Status.Resources, ssi.Status.Patches, ssi.Status.Secrets} {
		for _, status := range statuses {
			if applySuccessCondition := controllerutils.FindSyncCondition(status.Conditions, hivev1.ApplySuccessSyncCondition); applySuccessCondition != nil {
				since := time.Since(applySuccessCondition.LastProbeTime.Time)
				if since > timeSinceOldestApply {
					timeSinceOldestApply = since
				}
			}
		}
	}
	return reapplyInterval - timeSinceOldestApply
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

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)

	// If the cluster is unreachable, do not reconcile.
	if remoteClientBuilder.Unreachable() {
		ssiLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	dynamicClient, err := remoteClientBuilder.BuildDynamic()
	if err != nil {
		ssiLog.WithError(err).Error("unable to build dynamic client")
		return reconcile.Result{}, err
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

	if err := r.applySyncSetResources(ssi, spec.Resources, dynamicClient, h, ssiLog); err != nil {
		return err
	}
	if err := r.applySyncSetPatches(ssi, spec.Patches, h, ssiLog); err != nil {
		return err
	}
	return r.applySyncSetSecretMappings(ssi, spec.Secrets, dynamicClient, h, ssiLog)
}

func (r *ReconcileSyncSetInstance) deleteSyncSetResources(ssi *hivev1.SyncSetInstance, dynamicClient dynamic.Interface, ssiLog log.FieldLogger) error {
	var lastError error
	for index, resourceStatus := range ssi.Status.Resources {
		itemLog := ssiLog.WithField("resource", fmt.Sprintf("%s/%s", resourceStatus.Namespace, resourceStatus.Name)).
			WithField("apiversion", resourceStatus.APIVersion).
			WithField("kind", resourceStatus.Kind)
		gv, err := schema.ParseGroupVersion(resourceStatus.APIVersion)
		if err != nil {
			itemLog.WithError(err).Error("cannot parse resource apiVersion, skipping deletion")
			continue
		}
		gvr := gv.WithResource(resourceStatus.Resource)
		itemLog.Debug("deleting resource")
		err = dynamicClient.Resource(gvr).Namespace(resourceStatus.Namespace).Delete(resourceStatus.Name, &metav1.DeleteOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				itemLog.Debug("resource not found, nothing to do")
			case errors.IsForbidden(err):
				itemLog.WithError(err).Error("forbidden resource deletion, skipping")
			default:
				lastError = err
				itemLog.WithError(err).Error("error deleting resource")
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
			secretLog.WithError(err).Error("cannot parse secret apiVersion, skipping deletion")
			continue
		}
		gvr := gv.WithResource(secretStatus.Resource)
		secretLog.Debug("deleting secret")
		err = dynamicClient.Resource(gvr).Namespace(secretStatus.Namespace).Delete(secretStatus.Name, &metav1.DeleteOptions{})
		if err != nil {
			switch {
			case errors.IsNotFound(err):
				secretLog.Debug("secret not found, nothing to do")
			case errors.IsForbidden(err):
				secretLog.WithError(err).Error("forbidden secret deletion, skipping")
			default:
				lastError = err
				secretLog.WithError(err).Error("error deleting secret")
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
func (r *ReconcileSyncSetInstance) needToReapply(applyTerm string, newStatus, existingStatus hivev1.SyncStatus, ssiLog log.FieldLogger) bool {
	// Re-apply if hash has changed
	if existingStatus.Hash != newStatus.Hash {
		ssiLog.Debugf("%s %s/%s (%s) has changed, will re-apply", applyTerm, newStatus.Namespace, newStatus.Name, newStatus.Kind)
		return true
	}
	// Re-apply if failure occurred
	if failureCondition := controllerutils.FindSyncCondition(existingStatus.Conditions, hivev1.ApplyFailureSyncCondition); failureCondition != nil {
		if failureCondition.Status == corev1.ConditionTrue {
			ssiLog.Debugf("%s %s/%s (%s) failed last time, will re-apply", applyTerm, newStatus.Namespace, newStatus.Name, newStatus.Kind)
			return true
		}
	}
	// Re-apply if past reapplyInterval
	if applySuccessCondition := controllerutils.FindSyncCondition(existingStatus.Conditions, hivev1.ApplySuccessSyncCondition); applySuccessCondition != nil {
		since := time.Since(applySuccessCondition.LastProbeTime.Time)
		if since > r.reapplyInterval {
			ssiLog.Debugf("It has been %v since %s %s/%s (%s) was last applied, will re-apply", applyTerm, since, newStatus.Namespace, newStatus.Name, newStatus.Kind)
			return true
		}
	}
	return false
}

// applySyncSetResources evaluates resource objects from RawExtension and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetResources(ssi *hivev1.SyncSetInstance, resources []runtime.RawExtension, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	// determine if we can gather info for all resources
	infos := []hiveresource.Info{}
	for i, resource := range resources {
		info, err := h.Info(resource.Raw)
		if err != nil {
			ssi.Status.Conditions = r.setUnknownObjectSyncCondition(ssi.Status.Conditions, err, i)
			ssiLog.WithError(err).Warn("unable to parse resource")
			return err
		}
		infos = append(infos, *info)
	}

	ssi.Status.Conditions = r.clearUnknownObjectSyncCondition(ssi.Status.Conditions)
	syncStatusList := []hivev1.SyncStatus{}

	var applyErr error
	for i, resource := range resources {
		resourceSyncStatus := hivev1.SyncStatus{
			APIVersion: infos[i].APIVersion,
			Kind:       infos[i].Kind,
			Resource:   infos[i].Resource,
			Name:       infos[i].Name,
			Namespace:  infos[i].Namespace,
			Hash:       r.hash(resource.Raw),
		}

		if rss := findSyncStatus(resourceSyncStatus, ssi.Status.Resources); rss == nil || r.needToReapply("resource", resourceSyncStatus, *rss, ssiLog) {
			// Apply resource
			ssiLog.Debugf("applying resource: %s/%s (%s)", resourceSyncStatus.Namespace, resourceSyncStatus.Name, resourceSyncStatus.Kind)
			var applyResult hiveresource.ApplyResult
			applyResult, applyErr = h.Apply(resource.Raw)

			var resourceSyncConditions []hivev1.SyncCondition
			if rss != nil {
				resourceSyncConditions = rss.Conditions
			}
			resourceSyncStatus.Conditions = r.setApplySyncConditions(resourceSyncConditions, applyErr)

			if applyErr != nil {
				ssiLog.WithError(applyErr).Warnf("error applying resource %s/%s (%s)", resourceSyncStatus.Namespace, resourceSyncStatus.Name, resourceSyncStatus.Kind)
			} else {
				ssiLog.Debugf("resource %s/%s (%s): %s", resourceSyncStatus.Namespace, resourceSyncStatus.Name, resourceSyncStatus.Kind, applyResult)
			}
		} else {
			// Do not apply resource
			ssiLog.Debugf("resource %s/%s (%s) has not changed, will not apply", resourceSyncStatus.Namespace, resourceSyncStatus.Name, resourceSyncStatus.Kind)
			resourceSyncStatus.Conditions = rss.Conditions
		}

		syncStatusList = append(syncStatusList, resourceSyncStatus)

		// If an error applying occurred, stop processing right here
		if applyErr != nil {
			break
		}
	}

	ssi.Status.Resources = r.reconcileDeleted("resource", ssi.Spec.ResourceApplyMode, dynamicClient, ssi.Status.Resources, syncStatusList, applyErr, ssiLog)

	// Return applyErr for the controller to trigger retries and go into exponential backoff
	// if the problem does not resolve itself.
	if applyErr != nil {
		return applyErr
	}

	return nil
}

func (r *ReconcileSyncSetInstance) reconcileDeleted(deleteTerm string, applyMode hivev1.SyncSetResourceApplyMode, dynamicClient dynamic.Interface, existingStatusList, newStatusList []hivev1.SyncStatus, err error, ssiLog log.FieldLogger) []hivev1.SyncStatus {
	ssiLog.Debugf("reconciling syncset %ss, existing: %d, actual: %d", deleteTerm, len(existingStatusList), len(newStatusList))
	if applyMode == "" || applyMode == hivev1.UpsertResourceApplyMode {
		ssiLog.Debugf("apply mode is upsert, remote %ss will not be deleted", deleteTerm)
		return newStatusList
	}

	deletedStatusList := []hivev1.SyncStatus{}
	for _, existingStatus := range existingStatusList {
		if ss := findSyncStatus(existingStatus, newStatusList); ss == nil {
			ssiLog.WithField(deleteTerm, fmt.Sprintf("%s/%s", existingStatus.Namespace, existingStatus.Name)).
				WithField("apiversion", existingStatus.APIVersion).
				WithField("kind", existingStatus.Kind).Debugf("%s not found in updated status, will queue up for deletion", deleteTerm)
			deletedStatusList = append(deletedStatusList, existingStatus)
		}
	}

	// If an error occurred applying resources, do not delete yet
	if err != nil {
		ssiLog.Debugf("an error occurred applying %ss, will preserve all syncset status items", deleteTerm)
		return append(newStatusList, deletedStatusList...)
	}

	for _, deletedStatus := range deletedStatusList {
		itemLog := ssiLog.WithField(deleteTerm, fmt.Sprintf("%s/%s", deletedStatus.Namespace, deletedStatus.Name)).
			WithField("apiversion", deletedStatus.APIVersion).
			WithField("kind", deletedStatus.Kind)
		gv, err := schema.ParseGroupVersion(deletedStatus.APIVersion)
		if err != nil {
			itemLog.WithError(err).Errorf("unable to delete %s, cannot parse group version", deleteTerm)
			deletedStatus.Conditions = r.setDeletionFailedSyncCondition(deletedStatus.Conditions, err)
			newStatusList = append(newStatusList, deletedStatus)
			continue
		}
		gvr := gv.WithResource(deletedStatus.Resource)
		itemLog.Debugf("deleting %s", deleteTerm)
		err = dynamicClient.Resource(gvr).Namespace(deletedStatus.Namespace).Delete(deletedStatus.Name, &metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				itemLog.WithError(err).Warnf("error deleting %s", deleteTerm)
				deletedStatus.Conditions = r.setDeletionFailedSyncCondition(deletedStatus.Conditions, err)
				newStatusList = append(newStatusList, deletedStatus)
			} else {
				itemLog.Debugf("%s not found, nothing to do", deleteTerm)
			}
		}
	}

	return newStatusList
}

// applySyncSetPatches applies patches to cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetPatches(ssi *hivev1.SyncSetInstance, ssPatches []hivev1.SyncObjectPatch, h Applier, ssiLog log.FieldLogger) error {
	for _, ssPatch := range ssPatches {

		b, err := json.Marshal(ssPatch)
		if err != nil {
			ssiLog.WithError(err).Error("cannot serialize syncset patch")
			return err
		}
		patchSyncStatus := hivev1.SyncStatus{
			APIVersion: ssPatch.APIVersion,
			Kind:       ssPatch.Kind,
			Name:       ssPatch.Name,
			Namespace:  ssPatch.Namespace,
			Hash:       r.hash(b),
		}

		if pss := findSyncStatus(patchSyncStatus, ssi.Status.Patches); pss == nil || r.needToReapply("patch", patchSyncStatus, *pss, ssiLog) {
			// Apply patch
			ssiLog.Debugf("applying patch: %s/%s (%s)", ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
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
				ssiLog.WithError(err).Warnf("error applying patch: %s/%s (%s)", ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
				return err
			}
		} else {
			// Do not apply patch
			ssiLog.Debugf("patch %s/%s (%s) has not changed, will not apply", patchSyncStatus.Namespace, patchSyncStatus.Name, patchSyncStatus.Kind)
			patchSyncStatus.Conditions = pss.Conditions
		}
	}
	return nil
}

// applySyncSetSecretMappings evaluates secret mappings and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetSecretMappings(ssi *hivev1.SyncSetInstance, secretMappings []hivev1.SecretMapping, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	syncStatusList := []hivev1.SyncStatus{}

	var applyErr error
	for _, secretMapping := range secretMappings {
		secretSyncStatus := hivev1.SyncStatus{
			APIVersion: secretAPIVersion,
			Kind:       secretKind,
			Name:       secretMapping.TargetRef.Name,
			Namespace:  secretMapping.TargetRef.Namespace,
			Resource:   secretsResource,
		}

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
			ssiLog.WithError(applyErr).WithField("secret", fmt.Sprintf("%s/%s", secretMapping.SourceRef.Namespace, secretMapping.SourceRef.Name)).Log(logLevel, "cannot read secret")
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

		var hash string
		hash, applyErr = controllerutils.GetChecksumOfObject(secret)
		if applyErr != nil {
			ssiLog.WithError(applyErr).WithField("secret", fmt.Sprintf("%s/%s", secretMapping.SourceRef.Name, secretMapping.SourceRef.Namespace)).Error("unable to compute secret hash")
			secretSyncStatus.Conditions = r.setApplySyncConditions(secretSyncConditions, applyErr)
			syncStatusList = append(syncStatusList, secretSyncStatus)
			break
		}
		secretSyncStatus.Hash = hash

		if rss == nil || r.needToReapply("secret", secretSyncStatus, *rss, ssiLog) {
			// Apply secret
			ssiLog.Debugf("applying secret: %s/%s (%s)", secret.Namespace, secret.Name, secret.Kind)
			var result hiveresource.ApplyResult
			result, applyErr = h.ApplyRuntimeObject(secret, scheme.Scheme)

			var secretSyncConditions []hivev1.SyncCondition
			if rss != nil {
				secretSyncConditions = rss.Conditions
			}
			secretSyncStatus.Conditions = r.setApplySyncConditions(secretSyncConditions, applyErr)

			if applyErr != nil {
				ssiLog.WithError(applyErr).Warnf("error applying secret %s/%s (%s)", secret.Namespace, secret.Name, secret.Kind)
			} else {
				ssiLog.Debugf("resource %s/%s (%s): %s", secret.Namespace, secret.Name, secret.Kind, result)
			}
		} else {
			// Do not apply secret
			ssiLog.Debugf("resource %s/%s (%s) has not changed, will not apply", secret.Namespace, secret.Name, secret.Kind)
			secretSyncStatus.Conditions = rss.Conditions
		}

		syncStatusList = append(syncStatusList, secretSyncStatus)

		// If an error applying occurred, stop processing right here
		if applyErr != nil {
			break
		}
	}

	ssi.Status.Secrets = r.reconcileDeleted("secret", ssi.Spec.ResourceApplyMode, dynamicClient, ssi.Status.Secrets, syncStatusList, applyErr, ssiLog)

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

func (r *ReconcileSyncSetInstance) setUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition, err error, index int) []hivev1.SyncCondition {
	return controllerutils.SetSyncCondition(
		syncSetConditions,
		hivev1.UnknownObjectSyncCondition,
		corev1.ConditionTrue,
		unknownObjectFoundReason,
		fmt.Sprintf("Unable to gather Info for SyncSet resource at index %v in resources: %v", index, err),
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
}

func (r *ReconcileSyncSetInstance) clearUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition) []hivev1.SyncCondition {
	return controllerutils.SetSyncCondition(
		syncSetConditions,
		hivev1.UnknownObjectSyncCondition,
		corev1.ConditionFalse,
		unknownObjectFoundReason,
		fmt.Sprintf("Info available for all SyncSet resources"),
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
		controllerutils.UpdateConditionAlways)
}

func (r *ReconcileSyncSetInstance) resourceHash(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

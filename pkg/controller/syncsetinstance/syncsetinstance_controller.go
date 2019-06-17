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
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	hiveresource "github.com/openshift/hive/pkg/resource"
)

const (
	controllerName              = "syncsetinstance"
	unknownObjectFoundReason    = "UnknownObjectFound"
	unknownObjectNotFoundReason = "UnknownObjectNotFound"
	applySucceededReason        = "ApplySucceeded"
	applyFailedReason           = "ApplyFailed"
	deletionFailedReason        = "DeletionFailed"
	reapplyInterval             = 2 * time.Hour
)

// Applier knows how to Apply, Patch and return Info for []byte arrays describing objects and patches.
type Applier interface {
	Apply(obj []byte) (hiveresource.ApplyResult, error)
	Info(obj []byte) (*hiveresource.Info, error)
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType string) error
}

// Add creates a new SyncSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileSyncSetInstance{
		Client:               hivemetrics.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:               mgr.GetScheme(),
		logger:               log.WithField("controller", controllerName),
		applierBuilder:       applierBuilderFunc,
		dynamicClientBuilder: controllerutils.BuildDynamicClientFromKubeconfig,
	}
	r.hash = r.resourceHash
	return r
}

// applierBuilderFunc returns an Applier which implements Info, Apply and Patch
func applierBuilderFunc(kubeConfig []byte, logger log.FieldLogger) Applier {
	var helper Applier = hiveresource.NewHelper(kubeConfig, logger)
	return helper
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
	err := r.List(context.TODO(), &client.ListOptions{Namespace: cd.Namespace}, syncSetInstanceList)
	if err != nil {
		r.logger.WithError(err).Error("cannot list syncSetInstances for cluster deployment")
		return []reconcile.Request{}
	}

	retval := []reconcile.Request{}
	for _, syncSetInstance := range syncSetInstanceList.Items {
		if metav1.IsControlledBy(&syncSetInstance, cd) {
			retval = append(retval, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      cd.Name,
					Namespace: cd.Namespace,
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

	logger               log.FieldLogger
	applierBuilder       func([]byte, log.FieldLogger) Applier
	hash                 func([]byte) string
	dynamicClientBuilder func(string) (dynamic.Interface, error)
}

// Reconcile applies SyncSet or SelectorSyncSets associated with SyncSetInstances to the owning cluster.
// +kubebuilder:rbac:groups=hive.openshift.io,resources=syncsetinstances;syncsetinstances/status,verbs=get;create;update;delete;patch;list;watch
func (r *ReconcileSyncSetInstance) Reconcile(request reconcile.Request) (reconcile.Result, error) {
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

	ssiLog := r.logger.WithField("syncsetinstance", request.NamespacedName)

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

	if !cd.DeletionTimestamp.IsZero() {
		ssiLog.Debug("clusterdeployment is being deleted")
		return reconcile.Result{}, nil
	}

	if !cd.Status.Installed {
		ssiLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	// If the cluster is unreachable, do not reconcile.
	if controllerutils.HasUnreachableCondition(cd) {
		ssiLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	if len(cd.Status.AdminKubeconfigSecret.Name) == 0 {
		ssiLog.Debug("admin kubeconfig secret name is not set on clusterdeployment")
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
			ssiLog.WithError(err).Error("failed to delete syncsetinstance")
		}
		return reconcile.Result{}, err
	}

	// get kubeconfig for the cluster
	adminKubeconfigSecret, err := r.getKubeconfigSecret(cd, ssiLog)
	if err != nil {
		return reconcile.Result{}, err
	}
	kubeConfig, err := controllerutils.FixupKubeconfigSecretData(adminKubeconfigSecret.Data)
	if err != nil {
		ssiLog.WithError(err).Error("unable to fixup cluster client")
		return reconcile.Result{}, err
	}
	dynamicClient, err := r.dynamicClientBuilder(string(kubeConfig))
	if err != nil {
		ssiLog.WithError(err).Error("unable to build dynamic client")
		return reconcile.Result{}, err
	}
	ssiLog.Debug("applying sync set")
	original := ssi.DeepCopy()
	applier := r.applierBuilder(kubeConfig, ssiLog)
	applyErr := r.applySyncSet(ssi, spec, dynamicClient, applier, kubeConfig, ssiLog)
	err = r.updateSyncSetInstanceStatus(ssi, original, ssiLog)
	if err != nil {
		ssiLog.WithError(err).Errorf("error updating syncsetinstance status")
		return reconcile.Result{}, err
	}

	ssiLog.Info("done reconciling syncsetinstance")
	return reconcile.Result{}, applyErr
}

func (r *ReconcileSyncSetInstance) getClusterDeployment(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) (*hivev1.ClusterDeployment, error) {
	cd := &hivev1.ClusterDeployment{}
	cdName := types.NamespacedName{Namespace: ssi.Namespace, Name: ssi.Spec.ClusterDeployment.Name}
	ssiLog = ssiLog.WithField("clusterdeployment", cdName)
	err := r.Get(context.TODO(), cdName, cd)
	if err != nil {
		ssiLog.WithError(err).Error("error looking up clusterdeployment")
		return nil, err
	}
	return cd, nil
}

func (r *ReconcileSyncSetInstance) getKubeconfigSecret(cd *hivev1.ClusterDeployment, ssiLog log.FieldLogger) (*corev1.Secret, error) {
	if len(cd.Status.AdminKubeconfigSecret.Name) == 0 {
		return nil, fmt.Errorf("no kubeconfigconfig secret is set on clusterdeployment")
	}
	secret := &corev1.Secret{}
	secretName := types.NamespacedName{Name: cd.Status.AdminKubeconfigSecret.Name, Namespace: cd.Namespace}
	err := r.Get(context.TODO(), secretName, secret)
	if err != nil {
		ssiLog.WithError(err).WithField("secret", secretName).Error("unable to load admin kubeconfig secret")
		return nil, err
	}
	return secret, nil
}

func (r *ReconcileSyncSetInstance) addSyncSetInstanceFinalizer(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) error {
	ssiLog.Debug("adding finalizer")
	controllerutils.AddFinalizer(ssi, hivev1.FinalizerSyncSetInstance)
	err := r.Update(context.TODO(), ssi)
	if err != nil {
		ssiLog.WithError(err).Error("cannot add finalizer")
	}
	return err
}

func (r *ReconcileSyncSetInstance) removeSyncSetInstanceFinalizer(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) error {
	ssiLog.Debug("removing finalizer")
	controllerutils.DeleteFinalizer(ssi, hivev1.FinalizerSyncSetInstance)
	err := r.Update(context.TODO(), ssi)
	if err != nil {
		ssiLog.WithError(err).Error("cannot remove finalizer")
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

	// If the cluster is unreachable, do not reconcile.
	if controllerutils.HasUnreachableCondition(cd) {
		ssiLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	kubeconfigSecret, err := r.getKubeconfigSecret(cd, ssiLog)
	if errors.IsNotFound(err) {
		// kubeconfig secret cannot be found, just remove the finalizer
		return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
	} else if err != nil {
		// unknown error, try again
		return reconcile.Result{}, err
	}
	kubeConfig, err := controllerutils.FixupKubeconfigSecretData(kubeconfigSecret.Data)
	if err != nil {
		ssiLog.WithError(err).Error("unable to fixup cluster client")
		return reconcile.Result{}, err
	}
	dynamicClient, err := r.dynamicClientBuilder(string(kubeConfig))
	if err != nil {
		ssiLog.WithError(err).Error("unable to build dynamic client")
		return reconcile.Result{}, err
	}

	ssiLog.Info("deleting syncset resources on target cluster")
	err = r.deleteSyncSetResources(ssi, dynamicClient, ssiLog)
	if err == nil {
		return reconcile.Result{}, r.removeSyncSetInstanceFinalizer(ssi, ssiLog)
	}
	return reconcile.Result{}, err
}

func (r *ReconcileSyncSetInstance) applySyncSet(ssi *hivev1.SyncSetInstance, spec *hivev1.SyncSetCommonSpec, dynamicClient dynamic.Interface, h Applier, kubeConfig []byte, ssiLog log.FieldLogger) error {
	err := r.applySyncSetResources(ssi, spec.Resources, dynamicClient, h, ssiLog)
	if err != nil {
		ssiLog.WithError(err).Error("an error occurred applying syncset resources")
		return err
	}
	err = r.applySyncSetPatches(ssi, spec.Patches, kubeConfig, ssiLog)
	if err != nil {
		ssiLog.WithError(err).Error("an error occurred applying syncset patches")
		return err
	}
	return nil
}

func (r *ReconcileSyncSetInstance) deleteSyncSetResources(ssi *hivev1.SyncSetInstance, dynamicClient dynamic.Interface, ssiLog log.FieldLogger) error {
	var lastError error
	for index, resourceStatus := range ssi.Status.Resources {
		itemLog := ssiLog.WithField("resource", fmt.Sprintf("%s/%s", resourceStatus.Namespace, resourceStatus.Name)).
			WithField("apiversion", resourceStatus.APIVersion).
			WithField("kind", resourceStatus.Kind)
		gv, err := schema.ParseGroupVersion(resourceStatus.APIVersion)
		if err != nil {
			itemLog.WithError(err).Error("cannot parse resource apiVersion, skipping deletiong")
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

// getSyncSetCommonSpec returns the common spec of the associated syncset or selectorsyncset. It returns a boolean indicating
// whether the source object (syncset or selectorsyncset) has been deleted or is in the process of being deleted.
func (r *ReconcileSyncSetInstance) getSyncSetCommonSpec(ssi *hivev1.SyncSetInstance, ssiLog log.FieldLogger) (*hivev1.SyncSetCommonSpec, bool, error) {
	if ssi.Spec.SyncSet != nil {
		syncSet := &hivev1.SyncSet{}
		syncSetName := types.NamespacedName{Namespace: ssi.Namespace, Name: ssi.Spec.SyncSet.Name}
		err := r.Get(context.TODO(), syncSetName, syncSet)
		if errors.IsNotFound(err) || !syncSet.DeletionTimestamp.IsZero() {
			ssiLog.WithError(err).WithField("syncset", syncSetName).Warning("syncset is being deleted")
			return nil, true, nil
		}
		if err != nil {
			ssiLog.WithError(err).WithField("syncset", syncSetName).Error("cannot get associated syncset")
			return nil, false, err
		}
		if !syncSet.DeletionTimestamp.IsZero() {
			ssiLog.WithError(err).WithField("syncset", syncSetName).Warning("syncset is being deleted")
			return nil, false, nil
		}
		return &syncSet.Spec.SyncSetCommonSpec, false, nil
	} else if ssi.Spec.SelectorSyncSet != nil {
		selectorSyncSet := &hivev1.SelectorSyncSet{}
		selectorSyncSetName := types.NamespacedName{Name: ssi.Spec.SelectorSyncSet.Name}
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

// applySyncSetResources evaluates resource objects from RawExtension and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetResources(ssi *hivev1.SyncSetInstance, resources []runtime.RawExtension, dynamicClient dynamic.Interface, h Applier, ssiLog log.FieldLogger) error {
	// determine if we can gather info for all resources
	infos := []hiveresource.Info{}
	for i, resource := range resources {
		info, err := h.Info(resource.Raw)
		if err != nil {
			ssi.Status.Conditions = r.setUnknownObjectSyncCondition(ssi.Status.Conditions, err, i)
			return err
		}
		infos = append(infos, *info)
	}

	ssi.Status.Conditions = r.setUnknownObjectSyncCondition(ssi.Status.Conditions, nil, 0)
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

		var resourceSyncConditions []hivev1.SyncCondition

		// determine if resource is found, different or should be reapplied based on last probe time
		found := false
		different := false
		shouldReApply := false
		for _, rss := range ssi.Status.Resources {
			if rss.Name == resourceSyncStatus.Name &&
				rss.Namespace == resourceSyncStatus.Namespace &&
				rss.APIVersion == resourceSyncStatus.APIVersion &&
				rss.Kind == resourceSyncStatus.Kind {
				resourceSyncConditions = rss.Conditions
				found = true
				if rss.Hash != resourceSyncStatus.Hash {
					ssiLog.Debugf("Resource %s/%s (%s) has changed, will re-apply", infos[i].Namespace, infos[i].Name, infos[i].Kind)
					different = true
					break
				}

				// re-apply if failure occurred
				if failureCondition := controllerutils.FindSyncCondition(rss.Conditions, hivev1.ApplyFailureSyncCondition); failureCondition != nil {
					if failureCondition.Status == corev1.ConditionTrue {
						ssiLog.Debugf("Resource %s/%s (%s) failed last time, will re-apply", infos[i].Namespace, infos[i].Name, infos[i].Kind)
						shouldReApply = true
						break
					}
				}

				// re-apply if two hours have passed since LastProbeTime
				if applySuccessCondition := controllerutils.FindSyncCondition(rss.Conditions, hivev1.ApplySuccessSyncCondition); applySuccessCondition != nil {
					since := time.Since(applySuccessCondition.LastProbeTime.Time)
					if since > reapplyInterval {
						ssiLog.Debugf("It has been %v since resource %s/%s (%s) was last applied, will re-apply", since, infos[i].Namespace, infos[i].Name, infos[i].Kind)
						shouldReApply = true
					}
				}
				break
			}
		}

		if !found || different || shouldReApply {
			ssiLog.Debugf("applying resource: %s/%s (%s)", infos[i].Namespace, infos[i].Name, infos[i].Kind)
			var result hiveresource.ApplyResult
			result, applyErr = h.Apply(resource.Raw)
			resourceSyncStatus.Conditions = r.setApplySyncConditions(resourceSyncConditions, applyErr)
			if applyErr != nil {
				ssiLog.WithError(applyErr).Errorf("error applying resource %s/%s (%s)", infos[i].Namespace, infos[i].Name, infos[i].Kind)
			} else {
				ssiLog.Debugf("resource %s/%s (%s): %s", infos[i].Namespace, infos[i].Name, infos[i].Kind, result)
			}
		} else {
			ssiLog.Debugf("resource %s/%s (%s) has not changed, will not apply", infos[i].Namespace, infos[i].Name, infos[i].Kind)
			resourceSyncStatus.Conditions = resourceSyncConditions
		}

		syncStatusList = append(syncStatusList, resourceSyncStatus)

		// If an error applying occurred, stop processing right here
		if applyErr != nil {
			break
		}
	}

	var delErr error
	ssi.Status.Resources, delErr = r.reconcileDeletedSyncSetResources(ssi.Spec.ResourceApplyMode, dynamicClient, ssi.Status.Resources, syncStatusList, applyErr, ssiLog)
	if delErr != nil {
		ssiLog.WithError(delErr).Error("error reconciling syncset resources")
		return delErr
	}

	// We've saved apply and deletion errors separately, if either are present return an error for the controller to trigger retries
	// and go into exponential backoff if the problem does not resolve itself.
	if applyErr != nil {
		return applyErr
	}
	if delErr != nil {
		return delErr
	}

	return nil
}

// applySyncSetPatches applies patches to cluster identified by kubeConfig
func (r *ReconcileSyncSetInstance) applySyncSetPatches(ssi *hivev1.SyncSetInstance, ssPatches []hivev1.SyncObjectPatch, kubeConfig []byte, ssiLog log.FieldLogger) error {
	h := r.applierBuilder(kubeConfig, r.logger)

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

		patchSyncConditions := []hivev1.SyncCondition{}

		// determine if patch is found, different or should be reapplied based on patch apply mode
		found := false
		different := false
		shouldReApply := false
		for _, pss := range ssi.Status.Patches {
			if pss.Name == patchSyncStatus.Name && pss.Namespace == patchSyncStatus.Namespace && pss.Kind == patchSyncStatus.Kind {
				patchSyncConditions = pss.Conditions
				found = true
				if pss.Hash != patchSyncStatus.Hash {
					ssiLog.Debugf("Patch %s/%s (%s) has changed, will re-apply", ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
					different = true
					break
				}

				// re-apply if failure occurred
				if failureCondition := controllerutils.FindSyncCondition(pss.Conditions, hivev1.ApplyFailureSyncCondition); failureCondition != nil {
					if failureCondition.Status == corev1.ConditionTrue {
						ssiLog.Debugf("Patch %s/%s (%s) failed last time, will re-apply", ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
						shouldReApply = true
						break
					}
				}

				// re-apply if two hours have passed since LastProbeTime and patch apply mode is not apply once
				if ssPatch.ApplyMode != hivev1.ApplyOncePatchApplyMode {
					if applySuccessCondition := controllerutils.FindSyncCondition(pss.Conditions, hivev1.ApplySuccessSyncCondition); applySuccessCondition != nil {
						since := time.Since(applySuccessCondition.LastProbeTime.Time)
						if since > reapplyInterval {
							ssiLog.Debugf("It has been %v since resource %s/%s (%s) was last applied, will re-apply", since, ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
							shouldReApply = true
						}
					}
				}
				break
			}
		}

		if !found || different || shouldReApply {
			ssiLog.Debugf("applying patch: %s/%s (%s)", ssPatch.Namespace, ssPatch.Name, ssPatch.Kind)
			namespacedName := types.NamespacedName{
				Name:      ssPatch.Name,
				Namespace: ssPatch.Namespace,
			}
			err := h.Patch(namespacedName, ssPatch.Kind, ssPatch.APIVersion, []byte(ssPatch.Patch), ssPatch.PatchType)
			patchSyncStatus.Conditions = r.setApplySyncConditions(patchSyncConditions, err)
			ssi.Status.Patches = appendOrUpdateSyncStatus(ssi.Status.Patches, patchSyncStatus)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ReconcileSyncSetInstance) reconcileDeletedSyncSetResources(applyMode hivev1.SyncSetResourceApplyMode, dynamicClient dynamic.Interface, existingStatusList, newStatusList []hivev1.SyncStatus, err error, ssiLog log.FieldLogger) ([]hivev1.SyncStatus, error) {
	ssiLog.Debugf("reconciling syncset resources, existing: %d, actual: %d", len(existingStatusList), len(newStatusList))
	if applyMode == "" || applyMode == hivev1.UpsertResourceApplyMode {
		ssiLog.Debugf("apply mode is upsert, syncset status will be updated")
		return newStatusList, nil
	}
	deletedStatusList := []hivev1.SyncStatus{}
	deletedStatusIndices := []int{}
	for i, existingStatus := range existingStatusList {
		found := false
		for _, newStatus := range newStatusList {
			if existingStatus.Name == newStatus.Name &&
				existingStatus.Namespace == newStatus.Namespace &&
				existingStatus.APIVersion == newStatus.APIVersion &&
				existingStatus.Kind == newStatus.Kind {
				found = true
				break
			}
		}
		if !found {
			ssiLog.WithField("resource", fmt.Sprintf("%s/%s", existingStatus.Namespace, existingStatus.Name)).
				WithField("apiversion", existingStatus.APIVersion).
				WithField("kind", existingStatus.Kind).Debug("resource not found in updated status, will queue up for deletion")
			deletedStatusList = append(deletedStatusList, existingStatus)
			deletedStatusIndices = append(deletedStatusIndices, i)
		}
	}

	// If an error occurred applying resources, do not delete yet
	if err != nil {
		ssiLog.Debugf("an error occurred applying resources, will preserve all syncset status items")
		return append(newStatusList, deletedStatusList...), nil
	}

	for i, deletedStatus := range deletedStatusList {
		itemLog := ssiLog.WithField("resource", fmt.Sprintf("%s/%s", deletedStatus.Namespace, deletedStatus.Name)).
			WithField("apiversion", deletedStatus.APIVersion).
			WithField("kind", deletedStatus.Kind)
		gv, err := schema.ParseGroupVersion(deletedStatus.APIVersion)
		if err != nil {
			return nil, err
		}
		gvr := gv.WithResource(deletedStatus.Resource)
		itemLog.Debug("deleting resource")
		err = dynamicClient.Resource(gvr).Namespace(deletedStatus.Namespace).Delete(deletedStatus.Name, &metav1.DeleteOptions{})
		if err != nil {
			if !errors.IsNotFound(err) {
				itemLog.WithError(err).Error("error deleting resource")
				index := deletedStatusIndices[i]
				existingStatusList[index].Conditions = r.setDeletionFailedSyncCondition(existingStatusList[index].Conditions, err)
			} else {
				itemLog.Debug("resource not found, nothing to do")
			}
		}
	}

	return newStatusList, nil
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
		err := r.Status().Update(context.TODO(), ssi)
		if err != nil {
			ssiLog.WithError(err).Error("error updating syncsetinstance status")
			return err
		}
	}
	return nil
}

func (r *ReconcileSyncSetInstance) setUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition, err error, index int) []hivev1.SyncCondition {
	status := corev1.ConditionFalse
	reason := unknownObjectNotFoundReason
	message := fmt.Sprintf("Info available for all SyncSet resources")
	if err != nil {
		status = corev1.ConditionTrue
		reason = unknownObjectFoundReason
		message = fmt.Sprintf("Unable to gather Info for SyncSet resource at index %v in resources: %v", index, err)
	}
	syncSetConditions = controllerutils.SetSyncCondition(
		syncSetConditions,
		hivev1.UnknownObjectSyncCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionNever)
	return syncSetConditions
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
		// TODO: we cannot include the actual error here as it currently contains a temp filename which always changes,
		// which triggers a hotloop by always updating status and then reconciling again. If we were to filter out the portion
		// of the error message with filename, we could re-add this here.
		message = "Apply failed"
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

func (r *ReconcileSyncSetInstance) getRelatedSelectorSyncSets(cd *hivev1.ClusterDeployment) ([]hivev1.SelectorSyncSet, error) {
	list := &hivev1.SelectorSyncSetList{}
	err := r.Client.List(context.TODO(), &client.ListOptions{}, list)
	if err != nil {
		return nil, err
	}

	cdLabels := labels.Set(cd.Labels)
	selectorSyncSets := []hivev1.SelectorSyncSet{}
	for _, selectorSyncSet := range list.Items {
		labelSelector, err := metav1.LabelSelectorAsSelector(&selectorSyncSet.Spec.ClusterDeploymentSelector)
		if err != nil {
			r.logger.WithError(err).Error("unable to convert selector")
			continue
		}

		if labelSelector.Matches(cdLabels) {
			selectorSyncSets = append(selectorSyncSets, selectorSyncSet)
		}
	}

	return selectorSyncSets, err
}

func (r *ReconcileSyncSetInstance) getRelatedSyncSets(cd *hivev1.ClusterDeployment) ([]hivev1.SyncSet, error) {
	list := &hivev1.SyncSetList{}
	err := r.Client.List(context.TODO(), &client.ListOptions{Namespace: cd.Namespace}, list)
	if err != nil {
		return nil, err
	}

	syncSets := []hivev1.SyncSet{}
	for _, syncSet := range list.Items {
		for _, cdr := range syncSet.Spec.ClusterDeploymentRefs {
			if cdr.Name == cd.Name {
				syncSets = append(syncSets, syncSet)
				break
			}
		}
	}

	return syncSets, err
}

func (r *ReconcileSyncSetInstance) resourceHash(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}

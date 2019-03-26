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

package syncset

import (
	"context"
	"crypto/md5"
	"fmt"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	kapi "k8s.io/api/core/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName              = "syncset"
	adminKubeConfigKey          = "kubeconfig"
	unknownObjectFoundReason    = "UnknownObjectFound"
	unknownObjectNotFoundReason = "UnknownObjectNotFound"
	applySucceededReason        = "ApplySucceeded"
	applyFailedReason           = "ApplyFailed"
)

// Applier knows how to Apply, Patch and return Info for []byte arrays describing objects and patches.
type Applier interface {
	Apply(obj []byte) error
	Info(obj []byte) (*resource.Info, error)
	Patch(name types.NamespacedName, kind, apiVersion string, patch []byte, patchType types.PatchType) error
}

// Add creates a new SyncSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSyncSet{
		Client:         mgr.GetClient(),
		scheme:         mgr.GetScheme(),
		logger:         log.WithField("controller", controllerName),
		applierBuilder: applierBuilderFunc,
	}
}

// applierBuilderFunc returns an Applier which implements Info, Apply and Patch
func applierBuilderFunc(kubeConfig []byte, logger log.FieldLogger) Applier {
	var helper Applier = resource.NewHelper(kubeConfig, logger)
	return helper
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("syncset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for SyncSet
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSet{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(syncSetHandlerFunc),
	})
	if err != nil {
		return err
	}

	// Watch for SelectorSyncSet
	reconciler := r.(*ReconcileSyncSet)
	err = c.Watch(&source.Kind{Type: &hivev1.SelectorSyncSet{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(reconciler.selectorSyncSetHandlerFunc),
	})
	if err != nil {
		return err
	}

	return nil
}

func syncSetHandlerFunc(a handler.MapObject) []reconcile.Request {
	syncSet := a.Object.(*hivev1.SyncSet)
	retval := []reconcile.Request{}

	for _, clusterDeploymentRef := range syncSet.Spec.ClusterDeploymentRefs {
		retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      clusterDeploymentRef.Name,
			Namespace: syncSet.Namespace,
		}})
	}

	return retval
}

func (r *ReconcileSyncSet) selectorSyncSetHandlerFunc(a handler.MapObject) []reconcile.Request {
	selectorSyncSet := a.Object.(*hivev1.SelectorSyncSet)
	clusterDeployments := &hivev1.ClusterDeploymentList{}

	err := r.List(context.TODO(), &client.ListOptions{}, clusterDeployments)
	if err != nil {
		r.logger.WithError(err)
		return []reconcile.Request{}
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selectorSyncSet.Spec.ClusterDeploymentSelector)
	if err != nil {
		r.logger.WithError(err)
		return []reconcile.Request{}
	}

	retval := []reconcile.Request{}
	for _, clusterDeployment := range clusterDeployments.Items {
		if labelSelector.Matches(labels.Set(clusterDeployment.Labels)) {
			retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      clusterDeployment.Name,
				Namespace: clusterDeployment.Namespace,
			}})
		}
	}

	return retval
}

var _ reconcile.Reconciler = &ReconcileSyncSet{}

// ReconcileSyncSet reconciles a ClusterDeployment and the SyncSets associated with it
type ReconcileSyncSet struct {
	client.Client
	scheme *runtime.Scheme

	logger         log.FieldLogger
	applierBuilder func([]byte, log.FieldLogger) Applier
}

// Reconcile lists SyncSets and SelectorSyncSets which apply to a ClusterDeployment object and applies resources and patches
// found in each SyncSet object
// +kubebuilder:rbac:groups=hive.openshift.io,resources=selectorsyncsets,verbs=get;create;update;delete;patch;list;watch
func (r *ReconcileSyncSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}

	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		r.logger.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	if !cd.Status.Installed {
		// Cluster isn't installed yet, return
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	origCD := cd
	cd = cd.DeepCopy()

	cdLog.Info("reconciling sync sets for cluster deployment")

	// get all sync sets that apply to cd
	syncSets, err := r.getRelatedSyncSets(cd)
	if err != nil {
		cdLog.WithError(err).Error("unable to list related sync sets for cluster deployment")
		return reconcile.Result{}, err
	}

	// get all selector sync sets that apply to cd
	selectorSyncSets, err := r.getRelatedSelectorSyncSets(cd)
	if err != nil {
		cdLog.WithError(err).Error("unable to list related sync sets for cluster deployment")
		return reconcile.Result{}, err
	}

	// get kubeconfig for the cluster
	secretName := cd.Status.AdminKubeconfigSecret.Name
	secretData, err := r.loadSecretData(secretName, cd.Namespace, adminKubeConfigKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load admin kubeconfig")
		return reconcile.Result{}, err
	}
	kubeConfig := []byte(secretData)

	for _, syncSet := range syncSets {
		ssLog := cdLog.WithFields(log.Fields{"syncSet": syncSet.Name})
		ssLog.Debug("applying sync set")

		syncSetStatus := findSyncSetStatus(syncSet.Name, cd.Status.SyncSetStatus)
		err = r.applySyncSetResources(syncSet.Spec.Resources, kubeConfig, &syncSetStatus, ssLog)
		if err != nil {
			ssLog.WithError(err).Error("unable to apply sync set")
		}
		cd.Status.SyncSetStatus = appendOrUpdateSyncSetObjectStatus(cd.Status.SyncSetStatus, syncSetStatus)
	}

	for _, selectorSyncSet := range selectorSyncSets {
		ssLog := cdLog.WithFields(log.Fields{"selectorSyncSet": selectorSyncSet.Name})
		ssLog.Debug("applying selector sync set")

		syncSetStatus := findSyncSetStatus(selectorSyncSet.Name, cd.Status.SelectorSyncSetStatus)
		err = r.applySyncSetResources(selectorSyncSet.Spec.Resources, kubeConfig, &syncSetStatus, ssLog)
		if err != nil {
			ssLog.WithError(err).Error("unable to apply selector sync set")
		}
		cd.Status.SelectorSyncSetStatus = appendOrUpdateSyncSetObjectStatus(cd.Status.SelectorSyncSetStatus, syncSetStatus)
	}

	err = r.updateClusterDeploymentStatus(cd, origCD, cdLog)
	if err != nil {
		cdLog.WithError(err).Errorf("error updating cluster deployment status")
		return reconcile.Result{}, err
	}

	cdLog.Info("done reconciling sync sets for cluster deployment")

	return reconcile.Result{}, nil
}

// applySyncSetResources evaluates resource objects from RawExtension and applies them to the cluster identified by kubeConfig
func (r *ReconcileSyncSet) applySyncSetResources(ssResources []runtime.RawExtension, kubeConfig []byte, syncSetStatus *hivev1.SyncSetObjectStatus, ssLog log.FieldLogger) error {
	h := r.applierBuilder(kubeConfig, r.logger)

	// determine if we can gather info for all resources
	infos := []resource.Info{}
	for i, resource := range ssResources {
		info, err := h.Info(resource.Raw)
		if err != nil {
			// error gathering resource info, set UnknownObjectSyncCondition within syncSetStatus conditions
			syncSetStatus.Conditions = r.setUnknownObjectSyncCondition(syncSetStatus.Conditions, err, i)
			return err
		}
		infos = append(infos, *info)
	}

	syncSetStatus.Conditions = r.setUnknownObjectSyncCondition(syncSetStatus.Conditions, nil, 0)

	for i, resource := range ssResources {
		resourceHash := md5.Sum(resource.Raw)
		resourceSyncStatus := hivev1.SyncStatus{
			APIVersion: infos[i].APIVersion,
			Kind:       infos[i].Kind,
			Name:       infos[i].Name,
			Namespace:  infos[i].Namespace,
			Hash:       fmt.Sprintf("%x", resourceHash),
		}

		// determine if resource is found, different or should be reapplied based on last probe time
		found := false
		different := false
		shouldReApply := false
		for _, rss := range syncSetStatus.Resources {
			if rss.Name == resourceSyncStatus.Name && rss.Namespace == resourceSyncStatus.Namespace {
				found = true
				if rss.Hash != resourceSyncStatus.Hash {
					different = true
					break
				}

				// re-apply if two hours have passed since LastProbeTime
				applySuccessCondition := controllerutils.FindSyncCondition(rss.Conditions, hivev1.ApplySuccessSyncCondition)
				if applySuccessCondition != nil {
					now := metav1.Now()
					if now.Add(-2 * time.Hour).After(applySuccessCondition.LastProbeTime.Time) {
						shouldReApply = true
					}
				}
				break
			}
		}

		if !found || different || shouldReApply {
			ssLog.Debugf("applying resource: %v", infos[i].Name)
			err := h.Apply(resource.Raw)
			resourceSyncStatus.Conditions = r.setApplySuccessSyncCondition(resourceSyncStatus.Conditions, err)
			syncSetStatus.Resources = appendOrUpdateSyncStatus(syncSetStatus.Resources, resourceSyncStatus)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func appendOrUpdateSyncSetObjectStatus(statusList []hivev1.SyncSetObjectStatus, syncSetObjectStatus hivev1.SyncSetObjectStatus) []hivev1.SyncSetObjectStatus {
	found := false
	for _, ssos := range statusList {
		if ssos.Name == syncSetObjectStatus.Name {
			found = true
			ssos = syncSetObjectStatus
			break
		}
	}
	if !found {
		statusList = append(statusList, syncSetObjectStatus)
	}
	return statusList
}

func appendOrUpdateSyncStatus(statusList []hivev1.SyncStatus, syncStatus hivev1.SyncStatus) []hivev1.SyncStatus {
	found := false
	for i, ss := range statusList {
		if ss.Name == syncStatus.Name && ss.Namespace == syncStatus.Namespace && ss.Kind == syncStatus.Kind {
			found = true
			statusList[i] = syncStatus
			break
		}
	}
	if !found {
		statusList = append(statusList, syncStatus)
	}
	return statusList
}

func findSyncSetStatus(name string, statusList []hivev1.SyncSetObjectStatus) hivev1.SyncSetObjectStatus {
	syncSetStatus := hivev1.SyncSetObjectStatus{Name: name}
	for _, ssos := range statusList {
		if name == ssos.Name {
			syncSetStatus = ssos
			break
		}
	}
	return syncSetStatus
}

func (r *ReconcileSyncSet) updateClusterDeploymentStatus(cd *hivev1.ClusterDeployment, origCD *hivev1.ClusterDeployment, cdLog log.FieldLogger) error {
	// Update cluster deployment status if changed:
	if !reflect.DeepEqual(cd.Status, origCD.Status) {
		cdLog.Infof("status has changed, updating cluster deployment status")
		err := r.Status().Update(context.TODO(), cd)
		if err != nil {
			cdLog.Errorf("error updating cluster deployment status: %v", err)
			return err
		}
	}
	return nil
}

func (r *ReconcileSyncSet) setUnknownObjectSyncCondition(syncSetConditions []hivev1.SyncCondition, err error, index int) []hivev1.SyncCondition {
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

func (r *ReconcileSyncSet) setApplySuccessSyncCondition(resourceSyncConditions []hivev1.SyncCondition, err error) []hivev1.SyncCondition {
	status := corev1.ConditionTrue
	reason := applySucceededReason
	message := fmt.Sprintf("Apply successful")
	if err != nil {
		status = corev1.ConditionFalse
		reason = applyFailedReason
		message = fmt.Sprintf("Apply failed: %v", err)
	}
	resourceSyncConditions = controllerutils.SetSyncCondition(
		resourceSyncConditions,
		hivev1.ApplySuccessSyncCondition,
		status,
		reason,
		message,
		controllerutils.UpdateConditionAlways)
	return resourceSyncConditions
}

func (r *ReconcileSyncSet) getRelatedSelectorSyncSets(cd *hivev1.ClusterDeployment) ([]hivev1.SelectorSyncSet, error) {
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

func (r *ReconcileSyncSet) getRelatedSyncSets(cd *hivev1.ClusterDeployment) ([]hivev1.SyncSet, error) {
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

func (r *ReconcileSyncSet) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &kapi.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

package syncset

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName = "syncset"
)

// Add creates a new SyncSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	r := &ReconcileSyncSet{
		Client:      controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:      mgr.GetScheme(),
		logger:      log.WithField("controller", controllerName),
		computeHash: controllerutils.GetChecksumOfObject,
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

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return fmt.Errorf("cannot start watch on clusterdeployments: %v", err)
	}

	// Watch for SyncSet
	err = c.Watch(&source.Kind{Type: &hivev1.SyncSet{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(syncSetHandlerFunc),
	})
	if err != nil {
		return fmt.Errorf("cannot start watch on syncsets: %v", err)
	}

	// Watch for SelectorSyncSet
	reconciler := r.(*ReconcileSyncSet)
	err = c.Watch(&source.Kind{Type: &hivev1.SelectorSyncSet{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(reconciler.selectorSyncSetHandlerFunc),
	})
	if err != nil {
		return fmt.Errorf("cannot start watch on selectorsyncsets: %v", err)
	}

	return nil
}

func (r *ReconcileSyncSet) selectorSyncSetHandlerFunc(a handler.MapObject) []reconcile.Request {
	selectorSyncSet, ok := a.Object.(*hivev1.SelectorSyncSet)
	if !ok {
		r.logger.Warning("unexpected object, expected SelectorSyncSet")
		return []reconcile.Request{}
	}
	clusterDeployments := &hivev1.ClusterDeploymentList{}
	err := r.List(context.TODO(), clusterDeployments)
	if err != nil {
		r.logger.WithError(err).Error("cannot list cluster deployments for selector syncset")
		return []reconcile.Request{}
	}

	labelSelector, err := metav1.LabelSelectorAsSelector(&selectorSyncSet.Spec.ClusterDeploymentSelector)
	if err != nil {
		r.logger.WithError(err).
			WithField("selector", selectorSyncSet.Spec.ClusterDeploymentSelector).
			WithField("selectorsyncset", selectorSyncSet.Name).
			Error("cannot parse selector syncset label selector")
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

var _ reconcile.Reconciler = &ReconcileSyncSet{}

// ReconcileSyncSet reconciles a ClusterDeployment and the SyncSets associated with it
type ReconcileSyncSet struct {
	client.Client
	scheme      *runtime.Scheme
	logger      log.FieldLogger
	computeHash func(interface{}) (string, error)
}

// Reconcile lists SyncSets and SelectorSyncSets which apply to a ClusterDeployment object and applies resources and patches
// found in each SyncSet object
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
		r.logger.WithError(err).WithField("clusterDeployment", request.NamespacedName).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": request.NamespacedName,
	})

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		cdLog.Debug("clusterdeployment is being deleted, nothing to do")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

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

	syncSetInstances, err := r.getRelatedSyncSetInstances(cd)
	if err != nil {
		cdLog.WithError(err).Error("unable to list related sync set instances for cluster deployment")
		return reconcile.Result{}, err
	}

	toAdd, toUpdate, toDelete, err := r.reconcileSyncSetInstances(cd, syncSets, selectorSyncSets, syncSetInstances)
	if err != nil {
		cdLog.WithError(err).Error("unable to reconcile sync set instances for cluster deployment")
	}

	for _, syncSetInstance := range toUpdate {
		err := r.Update(context.TODO(), syncSetInstance)
		if err != nil {
			name := fmt.Sprintf("%s/%s", syncSetInstance.Namespace, syncSetInstance.Name)
			cdLog.WithError(err).WithField("syncSetInstance", name).Log(controllerutils.LogLevel(err), "cannot update sync set instance")
			return reconcile.Result{}, err
		}
	}

	for _, syncSetInstance := range toDelete {
		err := r.Delete(context.TODO(), syncSetInstance)
		if err != nil && !errors.IsNotFound(err) {
			name := fmt.Sprintf("%s/%s", syncSetInstance.Namespace, syncSetInstance.Name)
			cdLog.WithError(err).WithField("syncSetInstance", name).Log(controllerutils.LogLevel(err), "cannot delete sync set instance")
			return reconcile.Result{}, err
		}
	}

	for _, syncSetInstance := range toAdd {
		err := r.Create(context.TODO(), syncSetInstance)
		if err != nil {
			name := fmt.Sprintf("%s/%s", syncSetInstance.Namespace, syncSetInstance.Name)
			cdLog.WithError(err).WithField("syncSetInstance", name).Log(controllerutils.LogLevel(err), "cannot create sync set instance")
			return reconcile.Result{}, err
		}
	}
	cdLog.Info("done reconciling sync sets for cluster deployment")
	return reconcile.Result{}, nil
}

func (r *ReconcileSyncSet) reconcileSyncSetInstances(cd *hivev1.ClusterDeployment, syncSets []*hivev1.SyncSet, selectorSyncSets []*hivev1.SelectorSyncSet, syncSetInstances []*hivev1.SyncSetInstance) (
	toAdd []*hivev1.SyncSetInstance,
	toUpdate []*hivev1.SyncSetInstance,
	toDelete []*hivev1.SyncSetInstance,
	err error) {

	var computedHash string
	var syncSetInstance *hivev1.SyncSetInstance

	for _, syncSet := range syncSets {
		matchingSyncSetInstance := findSyncSetInstanceForSyncSet(syncSet.Name, syncSetInstances)
		if matchingSyncSetInstance == nil {
			syncSetInstance, err = r.syncSetInstanceForSyncSet(cd, syncSet)
			if err != nil {
				err = fmt.Errorf("cannot create syncset instance for syncset: %v", err)
				return
			}
			toAdd = append(toAdd, syncSetInstance)
			continue
		}
		computedHash, err = r.computeHash(syncSet.Spec)
		if err != nil {
			err = fmt.Errorf("cannot compute syncset hash: %v", err)
			return
		}
		if computedHash != matchingSyncSetInstance.Spec.SyncSetHash {
			matchingSyncSetInstance.Spec.SyncSetHash = computedHash
			matchingSyncSetInstance.Spec.ResourceApplyMode = syncSet.Spec.ResourceApplyMode
			toUpdate = append(toUpdate, matchingSyncSetInstance)
		}
	}

	for _, selectorSyncSet := range selectorSyncSets {
		matchingSyncSetInstance := findSyncSetInstanceForSelectorSyncSet(selectorSyncSet.Name, syncSetInstances)
		if matchingSyncSetInstance == nil {
			syncSetInstance, err = r.syncSetInstanceForSelectorSyncSet(cd, selectorSyncSet)
			if err != nil {
				err = fmt.Errorf("cannot create syncset instance for selectorsyncset: %v", err)
				return
			}
			toAdd = append(toAdd, syncSetInstance)
			continue
		}
		computedHash, err = r.computeHash(selectorSyncSet.Spec)
		if err != nil {
			err = fmt.Errorf("cannot comput selectorsyncset hash: %v", err)
			return
		}
		if computedHash != matchingSyncSetInstance.Spec.SyncSetHash {
			matchingSyncSetInstance.Spec.SyncSetHash = computedHash
			matchingSyncSetInstance.Spec.ResourceApplyMode = selectorSyncSet.Spec.ResourceApplyMode
			toUpdate = append(toUpdate, matchingSyncSetInstance)
		}
	}

	for _, syncSetInstance := range syncSetInstances {
		if syncSetInstance.Spec.SyncSet != nil {
			if !containsSyncSet(syncSetInstance.Spec.SyncSet.Name, syncSets) {
				toDelete = append(toDelete, syncSetInstance)
			}
		} else if syncSetInstance.Spec.SelectorSyncSet != nil {
			if !containsSelectorSyncSet(syncSetInstance.Spec.SelectorSyncSet.Name, selectorSyncSets) {
				toDelete = append(toDelete, syncSetInstance)
			}
		}
	}

	return
}

func (r *ReconcileSyncSet) getRelatedSelectorSyncSets(cd *hivev1.ClusterDeployment) ([]*hivev1.SelectorSyncSet, error) {
	list := &hivev1.SelectorSyncSetList{}
	err := r.Client.List(context.TODO(), list)
	if err != nil {
		return nil, err
	}

	cdLabels := labels.Set(cd.Labels)
	selectorSyncSets := []*hivev1.SelectorSyncSet{}
	for i, selectorSyncSet := range list.Items {
		if !selectorSyncSet.DeletionTimestamp.IsZero() {
			continue
		}
		labelSelector, err := metav1.LabelSelectorAsSelector(&selectorSyncSet.Spec.ClusterDeploymentSelector)
		if err != nil {
			r.logger.WithError(err).Error("unable to convert selector")
			continue
		}

		if labelSelector.Matches(cdLabels) {
			selectorSyncSets = append(selectorSyncSets, &list.Items[i])
		}
	}

	return selectorSyncSets, nil
}

func (r *ReconcileSyncSet) getRelatedSyncSets(cd *hivev1.ClusterDeployment) ([]*hivev1.SyncSet, error) {
	list := &hivev1.SyncSetList{}
	err := r.Client.List(context.TODO(), list, client.InNamespace(cd.Namespace))
	if err != nil {
		return nil, err
	}

	syncSets := []*hivev1.SyncSet{}
	for i, syncSet := range list.Items {
		if !syncSet.DeletionTimestamp.IsZero() {
			continue
		}
		for _, cdr := range syncSet.Spec.ClusterDeploymentRefs {
			if cdr.Name == cd.Name {
				syncSets = append(syncSets, &list.Items[i])
				break
			}
		}
	}

	return syncSets, nil
}

func (r *ReconcileSyncSet) getRelatedSyncSetInstances(cd *hivev1.ClusterDeployment) ([]*hivev1.SyncSetInstance, error) {
	list := &hivev1.SyncSetInstanceList{}
	err := r.Client.List(context.TODO(), list, client.InNamespace(cd.Namespace))
	if err != nil {
		return nil, err
	}

	syncSetInstances := []*hivev1.SyncSetInstance{}
	for i, syncSetInstance := range list.Items {
		if syncSetInstance.Spec.ClusterDeployment.Name == cd.Name {
			syncSetInstances = append(syncSetInstances, &list.Items[i])
		}
	}

	return syncSetInstances, nil
}

func findSyncSetInstanceForSyncSet(name string, syncSetInstances []*hivev1.SyncSetInstance) *hivev1.SyncSetInstance {
	for _, syncSetInstance := range syncSetInstances {
		if syncSetInstance.Spec.SyncSet != nil && syncSetInstance.Spec.SyncSet.Name == name {
			return syncSetInstance
		}
	}
	return nil
}

func findSyncSetInstanceForSelectorSyncSet(name string, syncSetInstances []*hivev1.SyncSetInstance) *hivev1.SyncSetInstance {
	for _, syncSetInstance := range syncSetInstances {
		if syncSetInstance.Spec.SelectorSyncSet != nil && syncSetInstance.Spec.SelectorSyncSet.Name == name {
			return syncSetInstance
		}
	}
	return nil
}

func (r *ReconcileSyncSet) syncSetInstanceForSyncSet(cd *hivev1.ClusterDeployment, syncSet *hivev1.SyncSet) (*hivev1.SyncSetInstance, error) {
	cdRef := metav1.NewControllerRef(cd, hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"))
	hash, err := r.computeHash(syncSet.Spec)
	if err != nil {
		return nil, err
	}
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:            syncSetInstanceNameForSyncSet(cd, syncSet),
			Namespace:       cd.Namespace,
			OwnerReferences: []metav1.OwnerReference{*cdRef},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeployment: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			SyncSet: &corev1.LocalObjectReference{
				Name: syncSet.Name,
			},
			ResourceApplyMode: syncSet.Spec.ResourceApplyMode,
			SyncSetHash:       hash,
		},
	}, nil
}

func (r *ReconcileSyncSet) syncSetInstanceForSelectorSyncSet(cd *hivev1.ClusterDeployment, selectorSyncSet *hivev1.SelectorSyncSet) (*hivev1.SyncSetInstance, error) {
	cdRef := metav1.NewControllerRef(cd, hivev1.SchemeGroupVersion.WithKind("ClusterDeployment"))
	hash, err := r.computeHash(selectorSyncSet.Spec)
	if err != nil {
		return nil, err
	}
	return &hivev1.SyncSetInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:            syncSetInstanceNameForSelectorSyncSet(cd, selectorSyncSet),
			Namespace:       cd.Namespace,
			OwnerReferences: []metav1.OwnerReference{*cdRef},
		},
		Spec: hivev1.SyncSetInstanceSpec{
			ClusterDeployment: corev1.LocalObjectReference{
				Name: cd.Name,
			},
			SelectorSyncSet: &hivev1.SelectorSyncSetReference{
				Name: selectorSyncSet.Name,
			},
			ResourceApplyMode: selectorSyncSet.Spec.ResourceApplyMode,
			SyncSetHash:       hash,
		},
	}, nil
}

func containsSyncSet(name string, syncSets []*hivev1.SyncSet) bool {
	for _, syncSet := range syncSets {
		if syncSet.Name == name {
			return true
		}
	}
	return false
}

func containsSelectorSyncSet(name string, selectorSyncSets []*hivev1.SelectorSyncSet) bool {
	for _, selectorSyncSet := range selectorSyncSets {
		if selectorSyncSet.Name == name {
			return true
		}
	}
	return false
}

func syncSetInstanceNameForSyncSet(cd *hivev1.ClusterDeployment, syncSet *hivev1.SyncSet) string {
	syncSetPart := helpers.GetName(syncSet.Name, "syncset", validation.DNS1123SubdomainMaxLength-validation.DNS1123LabelMaxLength)
	return fmt.Sprintf("%s-%s", cd.Name, syncSetPart)
}

func syncSetInstanceNameForSelectorSyncSet(cd *hivev1.ClusterDeployment, selectorSyncSet *hivev1.SelectorSyncSet) string {
	syncSetPart := helpers.GetName(selectorSyncSet.Name, "selector-syncset", validation.DNS1123SubdomainMaxLength-validation.DNS1123LabelMaxLength)
	return fmt.Sprintf("%s-%s", cd.Name, syncSetPart)
}

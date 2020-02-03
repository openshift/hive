package syncidentityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8slabels "k8s.io/kubernetes/pkg/util/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
)

const (
	controllerName = "syncidentityprovider"

	oauthAPIVersion = "config.openshift.io/v1"
	oauthKind       = "OAuth"
	oauthObjectName = "cluster"
)

// Add creates a new IdentityProvider Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSyncIdentityProviders{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", controllerName),
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName+"-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	reconciler := r.(*ReconcileSyncIdentityProviders)

	// Watch for changes to SyncIdentityProvider
	err = c.Watch(&source.Kind{Type: &hivev1.SyncIdentityProvider{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(reconciler.syncIdentityProviderWatchHandler),
	})
	if err != nil {
		return err
	}

	// Watch for changes to SelectorSyncIdentityProvider
	err = c.Watch(&source.Kind{Type: &hivev1.SelectorSyncIdentityProvider{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(reconciler.selectorSyncIdentityProviderWatchHandler),
	})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment (easy case)
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	return err
}

func (r *ReconcileSyncIdentityProviders) syncIdentityProviderWatchHandler(a handler.MapObject) []reconcile.Request {
	retval := []reconcile.Request{}

	syncIDP := a.Object.(*hivev1.SyncIdentityProvider)
	if syncIDP == nil {
		// Wasn't a SyncIdentityProvider, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to SyncIdentityProvider. Value: %+v", a.Object)
		return retval
	}

	for _, clusterDeploymentRef := range syncIDP.Spec.ClusterDeploymentRefs {
		retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
			Name:      clusterDeploymentRef.Name,
			Namespace: syncIDP.Namespace,
		}})
	}

	return retval
}

func (r *ReconcileSyncIdentityProviders) selectorSyncIdentityProviderWatchHandler(a handler.MapObject) []reconcile.Request {
	retval := []reconcile.Request{}

	ssidp := a.Object.(*hivev1.SelectorSyncIdentityProvider)
	if ssidp == nil {
		// Wasn't a SelectorSyncIdentityProvider, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to SelectorSyncIdentityProvider. Value: %+v", a.Object)
		return retval
	}

	contextLogger := addSelectorSyncIdentityProviderLoggerFields(r.logger, ssidp)

	clusterDeployments := &hivev1.ClusterDeploymentList{}
	r.List(context.TODO(), clusterDeployments)

	labelSelector, err := metav1.LabelSelectorAsSelector(&ssidp.Spec.ClusterDeploymentSelector)
	if err != nil {
		contextLogger.WithError(err).Error("Error converting LabelSelector to Selector")
		return retval
	}

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

var _ reconcile.Reconciler = &ReconcileSyncIdentityProviders{}

// ReconcileSyncIdentityProviders reconciles the MachineSets generated from a ClusterDeployment object
type ReconcileSyncIdentityProviders struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger
}

type identityProviderPatch struct {
	Spec identityProviderPatchSpec `json:"spec,omitempty"`
}

type identityProviderPatchSpec struct {
	IdentityProviders []openshiftapiv1.IdentityProvider `json:"identityProviders"`
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes to the
// remote cluster MachineSets based on the state read and the worker machines defined in
// ClusterDeployment.Spec.Config.Machines
func (r *ReconcileSyncIdentityProviders) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	contextLogger := addReconcileRequestLoggerFields(r.logger, request)

	// For logging, we need to see when the reconciliation loop starts and ends.
	contextLogger.Info("reconciling syncidentityproviders and clusterdeployments")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		contextLogger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			contextLogger.Info("cluster deployment not found")
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	contextLogger = addClusterDeploymentLoggerFields(contextLogger, cd)

	return reconcile.Result{}, r.syncIdentityProviders(cd)
}

func (r *ReconcileSyncIdentityProviders) createSyncSetSpec(cd *hivev1.ClusterDeployment, idps []openshiftapiv1.IdentityProvider) (*hivev1.SyncSetSpec, error) {
	idpPatch := identityProviderPatch{
		Spec: identityProviderPatchSpec{
			IdentityProviders: idps,
		},
	}

	patch, err := json.Marshal(idpPatch)
	if err != nil {
		return nil, fmt.Errorf("Failed marshaling identity provider list: %v", err)
	}

	return &hivev1.SyncSetSpec{
		ClusterDeploymentRefs: []corev1.LocalObjectReference{
			{
				Name: cd.Name,
			},
		},
		SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
			Patches: []hivev1.SyncObjectPatch{
				{
					APIVersion: oauthAPIVersion,
					Kind:       oauthKind,
					Name:       oauthObjectName,
					PatchType:  "merge",
					Patch:      string(patch),
				},
			},
		},
	}, nil
}

func (r *ReconcileSyncIdentityProviders) syncIdentityProviders(cd *hivev1.ClusterDeployment) error {
	contextLogger := addClusterDeploymentLoggerFields(r.logger, cd)

	idpsFromSSIDP, err := r.getRelatedSelectorSyncIdentityProviders(cd)
	if err != nil {
		return err
	}

	idpsFromSIDP, err := r.getRelatedSyncIdentityProviders(cd)
	if err != nil {
		return err
	}

	allIdps := append([]openshiftapiv1.IdentityProvider{}, idpsFromSSIDP...)
	allIdps = append(allIdps, idpsFromSIDP...)

	// Create a SyncSetSpec that includes all IdentityProviders as a patch
	newSyncSetSpec, err := r.createSyncSetSpec(cd, allIdps)
	if err != nil {
		return err
	}

	ssName := apihelpers.GetResourceName(cd.Name, "idp")

	ss := &hivev1.SyncSet{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: ssName, Namespace: cd.Namespace}, ss)
	if errors.IsNotFound(err) {
		if len(allIdps) == 0 {
			// The IDP list is empty -and- an existing syncset wasn't found, which means that IDPs on this cluster
			// haven't been managed previously. Therefore, DO NOT write out a syncset.
			contextLogger.Debug("IDP list empty and syncset not found. Not writing out syncset with empty IDP list.")
			return nil
		}

		ss = &hivev1.SyncSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ssName,
				Namespace: cd.Namespace,
			},
			Spec: *newSyncSetSpec,
		}

		// ensure the syncset gets cleaned up when the clusterdeployment is deleted
		r.logger.WithField("derivedObject", ss.Name).Debug("Setting labels on derived object")
		ss.Labels = k8slabels.AddLabel(ss.Labels, constants.ClusterDeploymentNameLabel, cd.Name)
		ss.Labels = k8slabels.AddLabel(ss.Labels, constants.SyncSetTypeLabel, constants.SyncSetTypeIdentityProvider)
		if err := controllerutil.SetControllerReference(cd, ss, r.scheme); err != nil {
			contextLogger.WithError(err).Error("error setting controller reference on syncset")
			return err
		}

		if err := r.Create(context.TODO(), ss); err != nil {
			contextLogger.WithError(err).Log(controllerutils.LogLevel(err), "error creating syncset")
			return err
		}

		// we successfully created it.
		return nil
	}

	if err != nil {
		contextLogger.WithError(err).Error("error checking for existing syncset")
		return err
	}

	// update the syncset if there have been changes
	if !reflect.DeepEqual(ss.Spec, *newSyncSetSpec) {
		ss.Spec = *newSyncSetSpec
		if err := r.Update(context.TODO(), ss); err != nil {
			errDetails := fmt.Errorf("error updating existing syncset: %v", err)
			contextLogger.Error(errDetails)
			return errDetails
		}
	}

	return nil
}

func (r *ReconcileSyncIdentityProviders) getRelatedSelectorSyncIdentityProviders(cd *hivev1.ClusterDeployment) ([]openshiftapiv1.IdentityProvider, error) {
	list := &hivev1.SelectorSyncIdentityProviderList{}
	err := r.Client.List(context.TODO(), list)
	if err != nil {
		return nil, err
	}

	contextLogger := addClusterDeploymentLoggerFields(r.logger, cd)

	cdLabelSet := labels.Set(cd.Labels)
	idps := []openshiftapiv1.IdentityProvider{}
	for _, ssidp := range list.Items {
		labelSelector, err := metav1.LabelSelectorAsSelector(&ssidp.Spec.ClusterDeploymentSelector)
		if err != nil {
			contextLogger.WithError(err).Error("error converting LabelSelector to Selector")
			continue
		}

		if labelSelector.Matches(cdLabelSet) {
			idps = append(idps, ssidp.Spec.IdentityProviders...)
		}
	}

	return idps, err
}

func (r *ReconcileSyncIdentityProviders) getRelatedSyncIdentityProviders(cd *hivev1.ClusterDeployment) ([]openshiftapiv1.IdentityProvider, error) {
	list := &hivev1.SyncIdentityProviderList{}
	err := r.Client.List(context.TODO(), list, client.InNamespace(cd.Namespace))
	if err != nil {
		return nil, err
	}

	idps := []openshiftapiv1.IdentityProvider{}
	for _, sip := range list.Items {
		for _, cdRef := range sip.Spec.ClusterDeploymentRefs {
			if cdRef.Name == cd.Name {
				idps = append(idps, sip.Spec.IdentityProviders...)
				break // This cluster deployment won't be listed twice in the ClusterDeploymentRefs
			}
		}
	}

	return idps, err
}

func addReconcileRequestLoggerFields(logger log.FieldLogger, request reconcile.Request) *log.Entry {
	return logger.WithFields(log.Fields{
		"NamespacedName": request.NamespacedName,
	})
}

func addClusterDeploymentLoggerFields(logger log.FieldLogger, cd *hivev1.ClusterDeployment) *log.Entry {
	return logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
}

func addSelectorSyncIdentityProviderLoggerFields(logger log.FieldLogger, ssidp *hivev1.SelectorSyncIdentityProvider) *log.Entry {
	return logger.WithFields(log.Fields{
		"Kind": ssidp.Kind,
		"Name": ssidp.Name,
	})
}

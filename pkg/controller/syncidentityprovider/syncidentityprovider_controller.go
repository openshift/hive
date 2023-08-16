package syncidentityprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	openshiftapiv1 "github.com/openshift/api/config/v1"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	k8slabels "github.com/openshift/hive/pkg/util/labels"
)

const (
	ControllerName = hivev1.SyncIdentityProviderControllerName

	oauthAPIVersion = "config.openshift.io/v1"
	oauthKind       = "OAuth"
	oauthObjectName = "cluster"
)

// Add creates a new IdentityProvider Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
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
	return &ReconcileSyncIdentityProviders{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", ControllerName),
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New(ControllerName.String()+"-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		return err
	}

	reconciler := r.(*ReconcileSyncIdentityProviders)

	// Watch for changes to SyncIdentityProvider
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.SyncIdentityProvider{}),
		handler.EnqueueRequestsFromMapFunc(reconciler.syncIdentityProviderWatchHandler))
	if err != nil {
		return err
	}

	// Watch for changes to SelectorSyncIdentityProvider
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.SelectorSyncIdentityProvider{}),
		handler.EnqueueRequestsFromMapFunc(reconciler.selectorSyncIdentityProviderWatchHandler))
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment (easy case)
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), &handler.EnqueueRequestForObject{})
	return err
}

func (r *ReconcileSyncIdentityProviders) syncIdentityProviderWatchHandler(ctx context.Context, a client.Object) []reconcile.Request {
	retval := []reconcile.Request{}

	syncIDP := a.(*hivev1.SyncIdentityProvider)
	if syncIDP == nil {
		// Wasn't a SyncIdentityProvider, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to SyncIdentityProvider. Value: %+v", a)
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

func (r *ReconcileSyncIdentityProviders) selectorSyncIdentityProviderWatchHandler(ctx context.Context, a client.Object) []reconcile.Request {
	retval := []reconcile.Request{}

	ssidp := a.(*hivev1.SelectorSyncIdentityProvider)
	if ssidp == nil {
		// Wasn't a SelectorSyncIdentityProvider, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to SelectorSyncIdentityProvider. Value: %+v", a)
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
func (r *ReconcileSyncIdentityProviders) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	contextLogger := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	contextLogger.Info("reconciling syncidentityproviders and clusterdeployments")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, contextLogger)
	defer recobsrv.ObserveControllerReconcileTime()

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
	contextLogger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, contextLogger)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		contextLogger.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Ensure owner references are correctly set
	err = controllerutils.ReconcileOwnerReferences(cd, generateOwnershipUniqueKeys(cd), r, r.scheme, contextLogger)
	if err != nil {
		contextLogger.WithError(err).Error("Error reconciling object ownership")
		return reconcile.Result{}, err
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	return reconcile.Result{}, r.syncIdentityProviders(cd, contextLogger)
}

func (r *ReconcileSyncIdentityProviders) createSyncSetSpec(cd *hivev1.ClusterDeployment, idps []openshiftapiv1.IdentityProvider) (*hivev1.SyncSetSpec, error) {
	idpPatch := identityProviderPatch{
		Spec: identityProviderPatchSpec{
			IdentityProviders: idps,
		},
	}

	patch, err := json.Marshal(idpPatch)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling identity provider list: %v", err)
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

// GenerateIdentityProviderSyncSetName generates the name of the SyncSet that holds the identity provider information to sync.
func GenerateIdentityProviderSyncSetName(clusterDeploymentName string) string {
	return apihelpers.GetResourceName(clusterDeploymentName, constants.IdentityProviderSuffix)
}

func (r *ReconcileSyncIdentityProviders) syncIdentityProviders(cd *hivev1.ClusterDeployment, contextLogger *log.Entry) error {
	idpsFromSSIDP, err := r.getRelatedSelectorSyncIdentityProviders(cd, contextLogger)
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

	ssName := GenerateIdentityProviderSyncSetName(cd.Name)

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

func (r *ReconcileSyncIdentityProviders) getRelatedSelectorSyncIdentityProviders(cd *hivev1.ClusterDeployment, contextLogger *log.Entry) ([]openshiftapiv1.IdentityProvider, error) {
	list := &hivev1.SelectorSyncIdentityProviderList{}
	err := r.Client.List(context.TODO(), list)
	if err != nil {
		return nil, err
	}

	cdLabelSet := labels.Set(cd.Labels)
	var idps []openshiftapiv1.IdentityProvider
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

	// Sort so that the patch is consistent
	idps = sortIdentityProviders(idps)

	return idps, err
}

func (r *ReconcileSyncIdentityProviders) getRelatedSyncIdentityProviders(cd *hivev1.ClusterDeployment) ([]openshiftapiv1.IdentityProvider, error) {
	list := &hivev1.SyncIdentityProviderList{}
	err := r.Client.List(context.TODO(), list, client.InNamespace(cd.Namespace))
	if err != nil {
		return nil, err
	}

	var idps []openshiftapiv1.IdentityProvider
	for _, sip := range list.Items {
		for _, cdRef := range sip.Spec.ClusterDeploymentRefs {
			if cdRef.Name == cd.Name {
				idps = append(idps, sip.Spec.IdentityProviders...)
				break // This cluster deployment won't be listed twice in the ClusterDeploymentRefs
			}
		}
	}

	// Sort so that the patch is consistent
	idps = sortIdentityProviders(idps)

	return idps, err
}

func addSelectorSyncIdentityProviderLoggerFields(logger log.FieldLogger, ssidp *hivev1.SelectorSyncIdentityProvider) *log.Entry {
	return logger.WithFields(log.Fields{
		"Kind": ssidp.Kind,
		"Name": ssidp.Name,
	})
}

func generateOwnershipUniqueKeys(owner hivev1.MetaRuntimeObject) []*controllerutils.OwnershipUniqueKey {
	return []*controllerutils.OwnershipUniqueKey{
		{
			TypeToList: &hivev1.SyncSetList{},
			LabelSelector: map[string]string{
				constants.ClusterDeploymentNameLabel: owner.GetName(),
				constants.SyncSetTypeLabel:           constants.SyncSetTypeIdentityProvider,
			},
			Controlled: true,
		},
	}
}

func sortIdentityProviders(idps []openshiftapiv1.IdentityProvider) []openshiftapiv1.IdentityProvider {
	sort.Slice(idps, func(i, j int) bool {
		return idps[i].Name < idps[j].Name
	})
	return idps
}

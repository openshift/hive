package clusterrelocate

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	ControllerName = hivev1.ClusterRelocateControllerName
)

var (
	typesToCopy = func() []runtime.Object {
		return []runtime.Object{
			&corev1.SecretList{},
			&corev1.ConfigMapList{},
			&hivev1.MachinePoolList{},
			&hivev1.SyncSetList{},
			&hivev1.SyncIdentityProviderList{},
		}
	}

	// clusterDeploymentClusterRelocateConditions are the cluster deployment conditions controlled by
	// Cluster Relocate controller
	clusterDeploymentClusterRelocateConditions = []hivev1.ClusterDeploymentConditionType{
		hivev1.RelocationFailedCondition,
	}

	metricSuccessfulClusterRelocations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_cluster_relocations",
		Help: "Total number of successful cluster relocations.",
	}, []string{"cluster_relocate"})

	metricAbortedClusterRelocations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "hive_aborted_cluster_relocations",
		Help: "Total number of aborted cluster relocations.",
	}, []string{"cluster_relocate", "reason"})

	// HIVE-2080: Ignore (do not copy) ConfigMaps laid down by the kube-controller-manager
	// in every namespace.
	ignoreConfigMapNames = sets.NewString("kube-root-ca.crt", "openshift-service-ca.crt")
)

func init() {
	metrics.Registry.MustRegister(metricSuccessfulClusterRelocations)
	metrics.Registry.MustRegister(metricAbortedClusterRelocations)
}

// Add creates a new ClusterRelocate controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}

	r := &ReconcileClusterRelocate{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &clientRateLimiter),
		logger: logger,
	}
	r.remoteClusterAPIClientBuilder = func(secret *corev1.Secret) remoteclient.Builder {
		return remoteclient.NewBuilderFromKubeconfig(r.Client, secret)
	}

	c, err := controller.New("clusterrelocate-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, r.logger),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             queueRateLimiter,
	})
	if err != nil {
		logger.WithError(err).Error("error creating controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}), &handler.EnqueueRequestForObject{}); err != nil {
		logger.WithError(err).Error("Error watching ClusterDeployment")
		return err
	}

	// Watch for changes to ClusterRelocate
	if err := c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterRelocate{}),
		handler.EnqueueRequestsFromMapFunc(r.clusterRelocateHandlerFunc)); err != nil {
		logger.WithError(err).Error("Error watching ClusterRelocate")
		return err
	}

	return nil
}

func (r *ReconcileClusterRelocate) clusterRelocateHandlerFunc(ctx context.Context, a client.Object) (requests []reconcile.Request) {
	clusterRelocate := a.(*hivev1.ClusterRelocate)

	labelSelector, err := metav1.LabelSelectorAsSelector(&clusterRelocate.Spec.ClusterDeploymentSelector)
	if err != nil {
		r.logger.WithError(err).
			WithField("clusterRelocate", clusterRelocate.Name).
			Warn("cannot parse clusterdeployment selector")
		return
	}

	clusterDeployments := &hivev1.ClusterDeploymentList{}
	if err := r.List(context.Background(), clusterDeployments); err != nil {
		r.logger.WithError(err).
			WithField("clusterRelocate", clusterRelocate.Name).
			Log(controllerutils.LogLevel(err), "failed to list clusterdeployments")
		return
	}

	for _, cd := range clusterDeployments.Items {
		if relocateName, _, _ := controllerutils.IsRelocating(&cd); relocateName != clusterRelocate.Name &&
			!labelSelector.Matches(labels.Set(cd.Labels)) {
			continue
		}
		requests = append(requests,
			reconcile.Request{NamespacedName: types.NamespacedName{Name: cd.Name, Namespace: cd.Namespace}})
	}

	return
}

// ReconcileClusterRelocate is the reconciler for ClusterRelocate. It will sync on ClusterDeployment resources
// and relocate those that match with a ClusterRelocate.
type ReconcileClusterRelocate struct {
	client.Client
	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(secret *corev1.Secret) remoteclient.Builder
}

// Reconcile relocates ClusterDeployments matching with a ClusterRelocate to another Hive instance.
func (r *ReconcileClusterRelocate) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "clusterDeployment", request.NamespacedName)
	logger.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Debug("cluster deployment not found")
			return reconcile.Result{}, nil
		}
		logger.WithError(err).Error("Error getting cluster deployment")
		return reconcile.Result{}, err
	}
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, logger)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		logger.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := controllerutils.InitializeClusterDeploymentConditions(cd.Status.Conditions,
		clusterDeploymentClusterRelocateConditions)
	if changed {
		cd.Status.Conditions = newConditions
		logger.Info("initializing cluster relocate controller conditions")
		if err := r.Status().Update(context.TODO(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update cluster deployment status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	currentRelocateName, relocateStatus, err := controllerutils.IsRelocating(cd)
	if err != nil {
		logger.WithError(err).Error("could not determine relocate status")
		return reconcile.Result{}, errors.Wrap(err, "could not determine relocate status")
	}

	// Complete the relocation on the destination side.
	// The ClusterDeployment is the last resource copied during a relocation. Since the ClusterDeployment is marked as
	// incoming, then this is the destination side of the relocation. As such, the relocation has been complete. All
	// that is left to do, on the destination side, is to remove the relocate annotations from the ClusterDeployment and
	// DNSZone.
	if relocateStatus == hivev1.RelocateIncoming {
		logger.Info("incoming relocation has completed")
		if err := r.stopRelocating(cd, currentRelocateName, logger); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to stop relocating")
		}
		// clear the current relocate name since the relocate has been completed and is no longer current
		currentRelocateName = ""
	}

	if cd.DeletionTimestamp != nil {
		// Stop relocating if the ClusterDeployment was deleted prior to completing the relocation
		if relocateStatus == hivev1.RelocateOutgoing {
			logger.Info("outgoing relocation aborted because ClusterDeployment was deleted")
			if err := r.stopRelocating(cd, currentRelocateName, logger); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to stop relocating")
			}
		} else {
			logger.Debug("skipping deleted clusterdeployment")
		}
		return reconcile.Result{}, err
	}

	// Skip any copying actions for ClusterDeployment that has already been relocated
	if relocateStatus == hivev1.RelocateComplete {
		return r.finishRelocateCompletion(cd, currentRelocateName, logger)
	}

	desiredRelocates, err := r.findMatchingRelocates(cd, logger)
	if err != nil {
		logger.WithError(err).Error("could not find matching relocates")
		return reconcile.Result{}, errors.Wrap(err, "could not find matching relocates")
	}

	// Do not do any relocation for a ClusterDeployment that does not match with exactly one ClusterRelocate
	if len(desiredRelocates) != 1 {
		return r.reconcileNoSingleMatch(cd, currentRelocateName, desiredRelocates, logger)
	}

	return r.reconcileSingleMatch(cd, relocateStatus, currentRelocateName, desiredRelocates[0], logger)
}

func (r *ReconcileClusterRelocate) reconcileSingleMatch(cd *hivev1.ClusterDeployment, oldRelocateStatus hivev1.RelocateStatus, oldRelocateName string, desiredRelocate *hivev1.ClusterRelocate, logger log.FieldLogger) (reconcile.Result, error) {
	// Abort an in-progress relocate when the ClusterDeployment matches a different ClusterRelocate
	if oldRelocateStatus == hivev1.RelocateOutgoing && oldRelocateName != desiredRelocate.Name {
		logger.WithField("currentClusterRelocate", oldRelocateName).
			WithField("desiredClusterRelocate", desiredRelocate.Name).
			Warn("switching relocation")
		if err := r.stopRelocating(cd, oldRelocateName, logger); err != nil {
			return reconcile.Result{}, err
		}
		recordMetricForAbortedRelocate(oldRelocateName, "new_match")
	}

	logger = logger.WithField("clusterRelocate", desiredRelocate.Name)

	kubeconfigSecret := &corev1.Secret{}
	if err := r.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: desiredRelocate.Spec.KubeconfigSecretRef.Namespace,
			Name:      desiredRelocate.Spec.KubeconfigSecretRef.Name,
		},
		kubeconfigSecret,
	); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get kubeconfig secret")
		r.setRelocationFailedCondition(
			cd,
			corev1.ConditionTrue,
			"MissingKubeconfigSecret",
			fmt.Sprintf("missing kubeconfig secret for destination cluster: %v", err),
			logger,
		)
		// return the error getting the kubeconfig secret rather than the update error
		return reconcile.Result{}, errors.Wrap(err, "failed to get kubeconfig secret")
	}

	destClient, err := r.remoteClusterAPIClientBuilder(kubeconfigSecret).Build()
	if err != nil {
		logger.WithError(err).Warn("could not create a client for the destination cluster")
		r.setRelocationFailedCondition(
			cd,
			corev1.ConditionTrue,
			"NoConnection",
			fmt.Sprintf("could not connect to destination cluster: %v", err),
			logger,
		)
		// return the error making the remote connection rather than the update error
		return reconcile.Result{}, errors.Wrap(err, "could not create a client for the destination cluster")
	}

	switch proceed, completed, err := r.checkForExistingClusterDeployment(cd, destClient, logger); {
	case err != nil:
		return reconcile.Result{}, err
	case completed:
		return r.finishRelocateCompletion(cd, desiredRelocate.Name, logger)
	case !proceed:
		return reconcile.Result{}, nil
	}

	if err := r.setRelocateAnnotation(cd, desiredRelocate.Name, hivev1.RelocateOutgoing, logger); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not set relocate status to outgoing")
	}

	// Copy resources to destination cluster
	if err := r.copy(cd, destClient, logger); err != nil {
		r.setRelocationFailedCondition(
			cd,
			corev1.ConditionTrue,
			"MoveFailed",
			err.Error(),
			logger,
		)
		// return the move error rather than the update error
		return reconcile.Result{}, err
	}

	return r.finishRelocateCompletion(cd, desiredRelocate.Name, logger)
}

// setRelocateAnnotation sets the relocate annotation on the ClusterDeployment as well as on the child DNSZone, if there
// is one.
func (r *ReconcileClusterRelocate) setRelocateAnnotation(cd *hivev1.ClusterDeployment, relocateName string, status hivev1.RelocateStatus, logger log.FieldLogger) error {
	dnsZone, err := r.dnsZone(cd, logger)
	if err != nil {
		return errors.Wrap(err, "failed to get DNSZone")
	}

	var objects []hivev1.MetaRuntimeObject
	if dnsZone != nil {
		// If setting to complete or clearing, do the DNSZone first. This ensures that if there is an error setting the
		// annotation on the DNSZone, that it will be re-attempting on the next reconcile. If the ClusterDeployment is set
		// first, then the next reconcile will not see that there is anything that needs to be done since the
		// ClusterDeployment will already have the annotation in the right state.
		if status == hivev1.RelocateComplete {
			objects = []hivev1.MetaRuntimeObject{dnsZone, cd}
		} else {
			objects = []hivev1.MetaRuntimeObject{cd, dnsZone}
		}
	} else {
		objects = []hivev1.MetaRuntimeObject{cd}
	}

	for _, obj := range objects {
		if changed := controllerutils.SetRelocateAnnotation(obj, relocateName, status); !changed {
			continue
		}
		if err := r.Update(context.Background(), obj); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to set relocate annotation")
			return errors.Wrap(err, "failed to set relocate annotation")
		}
	}
	return nil
}

// clearRelocateAnnotation deletes the relocate annotation from the ClusterDeployment as well as on the child DNSZone,
// if there is one.
func (r *ReconcileClusterRelocate) clearRelocateAnnotation(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	dnsZone, err := r.dnsZone(cd, logger)
	if err != nil {
		return errors.Wrap(err, "failed to get DNSZone")
	}
	objects := make([]hivev1.MetaRuntimeObject, 0, 2)
	if dnsZone != nil {
		objects = append(objects, dnsZone)
	}
	objects = append(objects, cd)

	for _, obj := range objects {
		if changed := controllerutils.ClearRelocateAnnotation(obj); !changed {
			continue
		}
		if err := r.Update(context.Background(), obj); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to clear relocate annotation")
			return errors.Wrap(err, "failed to clear relocate annotation")
		}
	}
	return nil
}

func (r *ReconcileClusterRelocate) finishRelocateCompletion(cd *hivev1.ClusterDeployment, relocateName string, logger log.FieldLogger) (reconcile.Result, error) {
	if err := r.setRelocateAnnotation(cd, relocateName, hivev1.RelocateComplete, logger); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not set relocate status to complete")
	}

	if err := r.setRelocationFailedCondition(
		cd,
		corev1.ConditionFalse,
		"MoveSuccessful",
		"move completed successfully",
		logger,
	); err != nil {
		return reconcile.Result{}, err
	}

	// Delete the ClusterDeployment since it has been successfully relocated to a new Hive instance
	if err := r.Delete(context.Background(), cd); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not delete relocated clusterdeployment")
		return reconcile.Result{}, errors.Wrap(err, "could not delete relocated clusterdeployment")
	}

	metricSuccessfulClusterRelocations.WithLabelValues(relocateName).Inc()

	return reconcile.Result{}, nil

}

func (r *ReconcileClusterRelocate) findMatchingRelocates(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]*hivev1.ClusterRelocate, error) {
	clusterRelocates := &hivev1.ClusterRelocateList{}
	if err := r.List(context.Background(), clusterRelocates); err != nil {
		return nil, errors.Wrap(err, "failed to list clusterrelocates")
	}
	var matches []*hivev1.ClusterRelocate
	for i, cr := range clusterRelocates.Items {
		labelSelector, err := metav1.LabelSelectorAsSelector(&cr.Spec.ClusterDeploymentSelector)
		if err != nil {
			r.logger.WithError(err).
				WithField("clusterRelocate", cr.Name).
				Warn("cannot parse clusterdeployment selector")
			continue
		}
		if labelSelector.Matches(labels.Set(cd.Labels)) {
			matches = append(matches, &clusterRelocates.Items[i])
		}
	}
	return matches, nil
}

func (r *ReconcileClusterRelocate) stopRelocating(cd *hivev1.ClusterDeployment, currentRelocateName string, logger log.FieldLogger) error {
	if currentRelocateName == "" {
		return nil
	}
	logger.WithField("clusterRelocate", currentRelocateName).Info("stopping relocation")
	// TODO: Attempt to clean up resources already relocated
	return r.clearRelocateAnnotation(cd, logger)
}

// reconcileNoSingleMatch reconciles a ClusterDeployment that does not match with exactly one ClusterRelocate.
// Any in-progress relocates will be aborted.
func (r *ReconcileClusterRelocate) reconcileNoSingleMatch(cd *hivev1.ClusterDeployment, currentRelocateName string, desiredRelocates []*hivev1.ClusterRelocate, logger log.FieldLogger) (reconcile.Result, error) {
	names := make([]string, len(desiredRelocates))
	for i, cr := range desiredRelocates {
		names[i] = cr.Name
	}
	logger = logger.WithField("matchingRelocates", names)
	if err := r.stopRelocating(cd, currentRelocateName, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to stop relocation")
		return reconcile.Result{}, errors.Wrap(err, "failed to stop relocation")
	}
	var (
		abortedReason   string
		status          corev1.ConditionStatus
		reason, message string
	)
	if len(desiredRelocates) == 0 {
		logger.Debug("no relocates match clusterdeployment")
		abortedReason = "no_match"
		status = corev1.ConditionFalse
		reason = "NoMatchingRelocates"
		message = "no ClusterRelocates match"
	} else {
		logger.Warn("cannot relocate clusterdeployment since it matches multiple clusterrelocates")
		abortedReason = "multiple_matches"
		status = corev1.ConditionTrue
		reason = "MultipleMatchingRelocates"
		message = fmt.Sprintf("multiple ClusterRelocates match: %s", strings.Join(names, ", "))
	}
	if err := r.setRelocationFailedCondition(cd, status, reason, message, logger); err != nil {
		return reconcile.Result{}, err
	}
	recordMetricForAbortedRelocate(currentRelocateName, abortedReason)
	return reconcile.Result{}, nil
}

// checkForExistingClusterDeployment checks whether the destination cluster already has the ClusterDeployment.
// proceed: true if the reconcile should proceed with copying resources to the destination cluster
// completed: true if the copy to the destination cluster has already been completed
// returnErr: non-nil if there was an error
//
// If the ClusterDeployment does not exist on the destination cluster, then proceed with copying the
// ClusterDeployment and its dependencies.
//
// If the ClusterDeployment exists on the destination cluster and is for the same base domain as the
// ClusterDeployment on the source cluster, then the copy is complete. This is due to the ClusterDeployment always
// being the last resource copied.
//
// If the ClusterDeployment exists on the destination cluster but is for a different base domain, then the
// ClusterDeployment on the destination cluster is in conflict with the ClusterDeployment on the source cluster.
// The ClusterDeployment on the destination cluster is for a separate Hive-managed cluster. There is nothing that
// Hive can or should do to resolve this. The ClusterDeployment cannot be relocated to the destination cluster
// unless the existing ClusterDeployment on the destination cluster is deleted.
func (r *ReconcileClusterRelocate) checkForExistingClusterDeployment(cd *hivev1.ClusterDeployment, destClient client.Client, logger log.FieldLogger) (proceed bool, completed bool, returnErr error) {
	cdKey := client.ObjectKeyFromObject(cd)
	destCD := &hivev1.ClusterDeployment{}
	switch err := destClient.Get(context.Background(), cdKey, destCD); {
	// no ClusterDeployment in destination cluster
	case apierrors.IsNotFound(err):
		logger.Info("clusterdeployment absent in destination cluster")
		proceed = true
	// error getting ClusterDeployment
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get clusterdeployment in destination cluster")
		returnErr = errors.Wrap(err, "failed to get clusterdeployment in destination cluster")
	// conflicting ClusterDeployment exists in destination cluster
	case destCD.Spec.BaseDomain != cd.Spec.BaseDomain:
		logger.Warn("clusterdeployment in destination cluster does not match the one being relocated")
		returnErr = r.setRelocationFailedCondition(
			cd,
			corev1.ConditionTrue,
			"ClusterDeploymentMismatch",
			"The ClusterDeployment in the destination cluster does not match the one being relocated. To relocate this ClusterDeployment, the ClusterDeployment in the destination cluster first must be deleted.",
			logger,
		)
	// ClusterDeployment already exists in destination cluster
	default:
		logger.Info("clusterdeployment already exists in destination cluster")
		// The ClusterDeployment must be the last resource copied. If the ClusterDeployment exists on the
		// destination cluster, then the copy is complete.
		completed = true
	}
	return
}

func (r *ReconcileClusterRelocate) copy(cd *hivev1.ClusterDeployment, destClient client.Client, logger log.FieldLogger) error {
	// create namespace
	switch err := destClient.Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: cd.Namespace,
		},
	}); {
	case apierrors.IsAlreadyExists(err):
		logger.Info("namespace already exists in destination cluster")
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to create namespace in destination cluster")
		return errors.Wrap(err, "failed to create namespace in destination cluster")
	default:
		logger.Info("namespace created")
	}

	// copy dependent resources
	for _, t := range typesToCopy() {
		if err := r.copyResources(cd, destClient, t.(client.ObjectList), logger); err != nil {
			return errors.Wrapf(err, "failed to copy %T", t)
		}
	}

	// copy dnszone
	dnsZone, err := r.dnsZone(cd, logger)
	if err != nil {
		return errors.Wrap(err, "could not get DNSZone")
	}
	if dnsZone != nil {
		logger = logger.WithField("type", reflect.TypeOf(dnsZone)).WithField("resource", dnsZone.Name)
		if err := r.copyResource(dnsZone, destClient, false, logger); err != nil {
			return errors.Wrap(err, "failed to copy dnszone")
		}
	}

	// copy clusterdeployment
	{
		logger := logger.WithField("type", reflect.TypeOf(cd)).WithField("resource", cd.Name)
		if err := r.copyResource(cd, destClient, true, logger); err != nil {
			return errors.Wrap(err, "failed to copy clusterdeployment")
		}
	}

	return nil
}

// copyResources copies all of the resources of the given object type in the namespace of the ClusterDeployment to the
// destination cluster
func (r *ReconcileClusterRelocate) copyResources(cd *hivev1.ClusterDeployment, destClient client.Client, objectList client.ObjectList, logger log.FieldLogger) error {
	logger = logger.WithField("type", reflect.TypeOf(objectList))
	if err := r.List(context.Background(), objectList, client.InNamespace(cd.Namespace)); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not list resources")
		return errors.Wrapf(err, "failed to list %T", objectList)
	}
	objs, err := meta.ExtractList(objectList)
	if err != nil {
		logger.WithError(err).Error("could not extract resources from list")
		return errors.Wrapf(err, "could not extract resources from %T", objectList)
	}
	for _, obj := range objs {
		logger := logger.WithField("type", reflect.TypeOf(obj))
		objMeta, err := meta.Accessor(obj)
		if err != nil {
			logger.WithError(err).Error("could not get object meta")
			return errors.Wrapf(err, "could not get object meta for %T", obj)
		}
		logger = logger.WithField("resource", objMeta.GetName())
		switch ignore, err := r.ignoreResource(obj, logger); {
		case err != nil:
			return errors.Wrap(err, "could not determine whether to ignore resource")
		case ignore:
			logger.Info("resource will not be copied since it is a resource that should be ignored")
			continue
		}
		if err := r.copyResource(obj, destClient, false, logger); err != nil {
			return errors.Wrapf(err, "could not copy %T resource %q", obj, objMeta.GetName())
		}
	}
	return nil
}

func (r *ReconcileClusterRelocate) copyResource(obj runtime.Object, destClient client.Client, failIfExists bool, logger log.FieldLogger) error {
	obj, err := prepareForComparison(obj)
	if err != nil {
		logger.WithError(err).Error("could not clear fields from source object")
		return errors.Wrap(err, "could not clear fields from source object")
	}
	// Need to use a copy here so that `obj` is left unaltered if the resource already exists on the remote cluster.
	objToCreate := obj.DeepCopyObject().(client.Object)
	switch err := destClient.Create(context.Background(), objToCreate); {
	case err == nil:
		logger.Info("resource created in destination cluster")
	case apierrors.IsAlreadyExists(err):
		if failIfExists {
			return errors.Wrap(err, "resource already exists in destination cluster")
		}
		logger.Info("resource already exists in destination cluster; replacing if there are changes")
		if err := r.replaceResourceIfChanged(destClient, obj.(client.Object), logger); err != nil {
			return errors.Wrap(err, "failed to sync existing resource")
		}
	default:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create resource in destination cluster")
		return errors.Wrap(err, "could not create resource in destination cluster")
	}
	return nil
}

func (r *ReconcileClusterRelocate) replaceResourceIfChanged(destClient client.Client, srcObj client.Object, logger log.FieldLogger) error {
	// Get the object from the destination cluster
	objKey := client.ObjectKeyFromObject(srcObj)
	destObj := reflect.New(reflect.TypeOf(srcObj).Elem()).Interface().(client.Object)
	if err := destClient.Get(context.Background(), objKey, destObj); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get resource from destination cluster")
		return errors.Wrap(err, "could not get resource from destination cluster")
	}

	// Prepare a copy of the object in the destination cluster for comparison with the object in the source cluster.
	clearedDestObj, err := prepareForComparison(destObj)
	if err != nil {
		logger.WithError(err).Error("could not clear fields of destination resource")
		return errors.Wrap(err, "could not clear fields of destination resource")
	}
	switch t := clearedDestObj.(type) {
	case *hivev1.ClusterDeployment:
		logger.Error("attempting to replace a ClusterDeployment")
		return errors.New("resource already exists in destination cluster")
	case *hivev1.MachinePool:
		// The remotemachineset controller in the destination cluster is going to remove its finalizer until the
		// ClusterDeployment exists in the destination cluster. This will cause the source and destination MachinePools
		// to have differences requiring a replacement if we do not set the finalizers equal first.
		t.Finalizers = srcObj.(*hivev1.MachinePool).Finalizers
	}

	// Check if there are any meaningful changes between the objects in the source and destination clusters
	if reflect.DeepEqual(srcObj, clearedDestObj) {
		// Do nothing. Resource in destination cluster is in sync.
		return nil
	}
	if logger.WithFields(nil).Logger.IsLevelEnabled(log.DebugLevel) {
		logger.WithField("diff", diff.ObjectReflectDiff(srcObj, clearedDestObj)).
			Debug("resource in destination cluster is out of sync")
	}

	// Delete the object in the destination cluster and re-create it.
	if err := destClient.Delete(context.Background(), destObj); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not delete out-of-sync resource from destination cluster")
		return errors.Wrap(err, "could not delete out-of-sync resource from destination cluster")
	}
	logger.Info("out-of-date resource deleted in destination cluster")
	if err := destClient.Create(context.Background(), srcObj); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create resource in destination cluster")
		return err
	}
	logger.Info("resource created in destination cluster")
	return nil
}

func prepareForComparison(obj runtime.Object) (runtime.Object, error) {
	obj = obj.DeepCopyObject()

	// Clear the GroupVersionKind in case there are version mismatches between the source cluster and destination cluster.
	obj.GetObjectKind().SetGroupVersionKind(schema.GroupVersionKind{})

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, errors.Wrap(err, "could not get object meta")
	}
	clearInstanceSpecificMeta(objMeta)

	switch t := obj.(type) {
	case *corev1.Secret, *corev1.ConfigMap:
		// Do nothing
	case *hivev1.MachinePool:
		t.Status = hivev1.MachinePoolStatus{}
	case *hivev1.SyncSet:
		t.Status = hivev1.SyncSetStatus{}
	case *hivev1.SyncIdentityProvider:
		t.Status = hivev1.IdentityProviderStatus{}
	case *hivev1.DNSZone:
		t.Status = hivev1.DNSZoneStatus{}
		if err := replaceOutgoingToIncoming(t); err != nil {
			return nil, errors.Wrap(err, "could not set relocate status to incoming")
		}
	case *hivev1.ClusterDeployment:
		t.Status = hivev1.ClusterDeploymentStatus{}
		if err := replaceOutgoingToIncoming(t); err != nil {
			return nil, errors.Wrap(err, "could not set relocate status to incoming")
		}
	default:
		return nil, errors.Errorf("unknown type to relocate: %T", t)
	}

	return obj, nil
}

func clearInstanceSpecificMeta(to metav1.Object) {
	to.SetSelfLink("")
	to.SetUID("")
	to.SetResourceVersion("")
	to.SetGeneration(0)
	to.SetCreationTimestamp(metav1.Time{})
	to.SetOwnerReferences(nil)
	to.SetManagedFields(nil)
}

// replaceOutgoingToIncoming changes the relocate status from outgoing to incoming. This is used when copying a resource
// to a destination cluster. On the source cluster, the status is outgoing. On the destination cluster, the status is
// incoming.
func replaceOutgoingToIncoming(obj hivev1.MetaRuntimeObject) error {
	relocateName, relocateStatus, err := controllerutils.IsRelocating(obj)
	if err != nil {
		return errors.Wrap(err, "could not determine relocate status")
	}
	switch relocateStatus {
	case hivev1.RelocateOutgoing:
		controllerutils.SetRelocateAnnotation(obj, relocateName, hivev1.RelocateIncoming)
	case hivev1.RelocateIncoming:
		// Do nothing as it is already incoming
	default:
		return errors.Errorf("resource is not relocating out: status=%q", relocateStatus)
	}
	return nil
}

// ignoreResource determines whether a resource should be ignored during a copy to a destination cluster. An ignored
// resource will not be copied.
func (r *ReconcileClusterRelocate) ignoreResource(obj runtime.Object, logger log.FieldLogger) (bool, error) {
	switch t := obj.(type) {
	case *corev1.ConfigMap:
		return r.ignoreConfigMap(t, logger)
	case *corev1.Secret:
		return r.ignoreSecret(t, logger)
	default:
		return false, nil
	}
}

func (r *ReconcileClusterRelocate) ignoreConfigMap(cm *corev1.ConfigMap, logger log.FieldLogger) (bool, error) {
	return ignoreConfigMapNames.Has(cm.Name), nil
}

// ignoreSecret determines whether a secret should be ignored during a copy to a destination cluster. An ignored
// secret will not be copied.
func (r *ReconcileClusterRelocate) ignoreSecret(secret *corev1.Secret, logger log.FieldLogger) (bool, error) {
	if secret.Type == corev1.SecretTypeServiceAccountToken {
		logger.Info("ignoring service account token secret")
		return true, nil
	}
	for _, ownerRef := range secret.OwnerReferences {
		if ownerRef.Kind != "Secret" {
			continue
		}
		owner := &corev1.Secret{}
		if err := r.Get(context.Background(), client.ObjectKey{Namespace: secret.Namespace, Name: ownerRef.Name}, owner); err != nil {
			logger.WithField("owner", ownerRef.Name).WithError(err).Log(controllerutils.LogLevel(err), "could not get owner secret")
			return false, err
		}
		if owner.Type == corev1.SecretTypeServiceAccountToken {
			logger.Info("ignoring secret owned by a service account token secret")
			return true, nil
		}
	}
	return false, nil
}

// dnsZone gets the DNSZone that is a child of the ClusterDeployment for the purposes of managed DNS.
// If the ClusterDeployment is not using managed DNS or the child DNSZone does not exist, the return will be nil.
func (r *ReconcileClusterRelocate) dnsZone(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (*hivev1.DNSZone, error) {
	if !cd.Spec.ManageDNS {
		return nil, nil
	}
	dnsZone := &hivev1.DNSZone{}
	switch err := r.Get(
		context.Background(),
		client.ObjectKey{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)},
		dnsZone,
	); {
	case apierrors.IsNotFound(err):
		logger.Info("no DNSZone found for ClusterDeployment with managed DNS")
		return nil, nil
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get DNSZone")
		return nil, err
	default:
		return dnsZone, nil
	}
}

func (r *ReconcileClusterRelocate) setRelocationFailedCondition(cd *hivev1.ClusterDeployment, status corev1.ConditionStatus, reason, message string, logger log.FieldLogger) error {
	updateConditionCheck := controllerutils.UpdateConditionIfReasonOrMessageChange
	if status == corev1.ConditionFalse {
		updateConditionCheck = controllerutils.UpdateConditionNever
	}
	conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.RelocationFailedCondition,
		status,
		reason,
		message,
		updateConditionCheck,
	)
	if !changed {
		return nil
	}
	cd.Status.Conditions = conds
	if err := r.Status().Update(context.Background(), cd); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update conditions")
		return errors.Wrap(err, "could not update conditions")
	}
	return nil
}

func recordMetricForAbortedRelocate(abortedRelocate, abortedReason string) {
	if abortedReason == "" {
		return
	}
	metricAbortedClusterRelocations.WithLabelValues(abortedRelocate, abortedReason).Inc()
}

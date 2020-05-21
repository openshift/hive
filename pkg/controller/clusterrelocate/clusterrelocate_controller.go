package clusterrelocate

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/diff"
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
)

const (
	controllerName = "clusterRelocate"
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

	metricSuccessfulClusterRelocations = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_cluster_relocations",
		Help: "Total number of successful cluster relocations.",
	}, []string{"cluster_relocate"})

	metricAbortedClusterRelocations = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "hive_aborted_cluster_relocations",
		Help: "Total number of aborted cluster relocations.",
	}, []string{"cluster_relocate", "reason"})
)

func init() {
	metrics.Registry.MustRegister(metricSuccessfulClusterRelocations)
	metrics.Registry.MustRegister(metricAbortedClusterRelocations)
}

// Add creates a new ClusterRelocate controller and adds it to the manager with default RBAC.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", controllerName)
	r := &ReconcileClusterRelocate{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
		logger: logger,
	}
	r.remoteClusterAPIClientBuilder = func(secret *corev1.Secret) remoteclient.Builder {
		return remoteclient.NewBuilderFromKubeconfig(r.Client, secret)
	}

	c, err := controller.New("clustereelocate-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		logger.WithError(err).Error("error creating controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{}); err != nil {
		logger.WithError(err).Error("Error watching ClusterDeployment")
		return err
	}

	// Watch for changes to ClusterRelocate
	if err := c.Watch(&source.Kind{Type: &hivev1.ClusterRelocate{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(r.clusterRelocateHandlerFunc),
	}); err != nil {
		logger.WithError(err).Error("Error watching ClusterRelocate")
		return err
	}

	return nil
}

func (r *ReconcileClusterRelocate) clusterRelocateHandlerFunc(a handler.MapObject) (requests []reconcile.Request) {
	clusterRelocate := a.Object.(*hivev1.ClusterRelocate)

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
		if cd.Annotations[constants.RelocatingAnnotation] != clusterRelocate.Name &&
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
	scheme *runtime.Scheme
	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(secret *corev1.Secret) remoteclient.Builder
}

// Reconcile relocates ClusterDeployments matching with a ClusterRelocate to another Hive instance.
func (r *ReconcileClusterRelocate) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithFields(log.Fields{
		"controller":        controllerName,
		"clusterDeployment": request.NamespacedName.String(),
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	logger.Info("reconciling cluster deployment")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

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

	if cd.DeletionTimestamp != nil {
		logger.Debug("skipping deleted clusterdeployment")
		return reconcile.Result{}, err
	}

	desiredRelocators, err := r.findMatchingRelocators(cd, logger)

	if len(desiredRelocators) != 1 {
		return r.reconcileNoSingleMatch(cd, desiredRelocators, logger)
	}
	desiredRelocator := desiredRelocators[0]

	currentRelocatorName, relocating := cd.Annotations[constants.RelocatingAnnotation]

	if relocating && currentRelocatorName != desiredRelocator.Name {
		logger.WithField("currentClusterRelocate", currentRelocatorName).
			WithField("desiredClusterRelocate", desiredRelocator.Name).
			Info("switching relocation")
		abortedReconcile, err := r.stopRelocating(cd, false, logger)
		if err != nil {
			return reconcile.Result{}, err
		}
		recordMetricForAbortedRelocate(abortedReconcile, "new_match")
	}

	return reconcile.Result{}, r.relocate(cd, desiredRelocator, logger)
}

func (r *ReconcileClusterRelocate) findMatchingRelocators(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]*hivev1.ClusterRelocate, error) {
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

func (r *ReconcileClusterRelocate) stopRelocating(cd *hivev1.ClusterDeployment, commitUpdates bool, logger log.FieldLogger) (abortedRelocate string, returnErr error) {
	clusterRelocateName, relocating := cd.Annotations[constants.RelocatingAnnotation]
	if !relocating {
		return
	}
	abortedRelocate = clusterRelocateName
	logger.WithField("clusterRelocate", clusterRelocateName).Info("stopping relocation")
	// TODO: Attempt to clean up resources already relocated
	delete(cd.Annotations, constants.RelocatingAnnotation)
	if commitUpdates {
		if err := r.Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to clear relocating annotation")
			returnErr = err
			return
		}
	}
	return
}

func (r *ReconcileClusterRelocate) reconcileNoSingleMatch(cd *hivev1.ClusterDeployment, desiredRelocators []*hivev1.ClusterRelocate, logger log.FieldLogger) (reconcile.Result, error) {
	names := make([]string, len(desiredRelocators))
	for i, cr := range desiredRelocators {
		names[i] = cr.Name
	}
	logger = logger.WithField("matchingRelocators", names)
	abortedRelocate, err := r.stopRelocating(cd, true, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to stop relocation")
		return reconcile.Result{}, errors.Wrap(err, "failed to stop relocation")
	}
	var abortedReason string
	if len(desiredRelocators) == 0 {
		logger.Debug("no relocators match clusterdeployment")
		abortedReason = "no_match"
		if conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.RelocationFailedCondition,
			corev1.ConditionFalse,
			"NoMatchingRelocators",
			"no ClusterRelocates match",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		); changed {
			cd.Status.Conditions = conds
			if err := r.Status().Update(context.Background(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update conditions")
				return reconcile.Result{}, errors.Wrap(err, "could not update conditions")
			}
		}
	} else {
		logger.Warn("cannot relocate clusterdeployment since it matches multiple clusterrelocates")
		abortedReason = "multiple_matches"
		if conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.RelocationFailedCondition,
			corev1.ConditionTrue,
			"MultipleMatchingRelocators",
			fmt.Sprintf("multiple ClusterRelocates match: %s", strings.Join(names, ", ")),
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		); changed {
			cd.Status.Conditions = conds
			if err := r.Status().Update(context.Background(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update conditions")
				return reconcile.Result{}, errors.Wrap(err, "could not update conditions")
			}
		}
	}
	recordMetricForAbortedRelocate(abortedRelocate, abortedReason)
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterRelocate) relocate(cd *hivev1.ClusterDeployment, relocator *hivev1.ClusterRelocate, logger log.FieldLogger) error {
	logger = logger.WithField("clusterRelocate", relocator.Name)

	if _, relocated := cd.Annotations[constants.RelocatedAnnotation]; !relocated {
		if cd.Annotations[constants.RelocatingAnnotation] != relocator.Name {
			if cd.Annotations == nil {
				cd.Annotations = make(map[string]string, 1)
			}
			cd.Annotations[constants.RelocatingAnnotation] = relocator.Name
			if err := r.Update(context.Background(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to add relocating annotation")
				return errors.Wrap(err, "failed to add relocation annotation")
			}
		}

		if completed, err := r.copy(cd, relocator, logger); err != nil {
			if conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
				cd.Status.Conditions,
				hivev1.RelocationFailedCondition,
				corev1.ConditionTrue,
				"MoveFailed",
				err.Error(),
				controllerutils.UpdateConditionIfReasonOrMessageChange,
			); changed {
				cd.Status.Conditions = conds
				if err := r.Status().Update(context.Background(), cd); err != nil {
					logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update condition")
					// return the move error rather than the update error
				}
			}
			return err
		} else if !completed {
			return nil
		}

		delete(cd.Annotations, constants.RelocatingAnnotation)
		cd.Annotations[constants.RelocatedAnnotation] = relocator.Name
		if err := r.Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to add relocated annotation")
			return errors.Wrap(err, "failed to add relocated annotation")
		}
	}

	if conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
		cd.Status.Conditions,
		hivev1.RelocationFailedCondition,
		corev1.ConditionFalse,
		"MoveSuccessful",
		"move completed successfully",
		controllerutils.UpdateConditionNever,
	); changed {
		cd.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), cd); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update condition")
			return errors.Wrap(err, "could not update condition")
		}
	}

	if err := r.Delete(context.Background(), cd); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not delete relocated clusterdeployment")
		return errors.Wrap(err, "could not delete relocated clusterdeployment")
	}

	return nil
}

func (r *ReconcileClusterRelocate) copy(cd *hivev1.ClusterDeployment, relocator *hivev1.ClusterRelocate, logger log.FieldLogger) (completed bool, err error) {
	kubeconfigSecret := &corev1.Secret{}
	if err := r.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: relocator.Spec.KubeconfigSecretRef.Namespace,
			Name:      relocator.Spec.KubeconfigSecretRef.Name,
		},
		kubeconfigSecret,
	); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get kubeconfig secret")
		return false, errors.Wrap(err, "failed to get kubeconfig secret")
	}

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(kubeconfigSecret)
	destClient, err := remoteClientBuilder.Build()
	if err != nil {
		logger.WithError(err).Warn("could not create a client for the destination cluster")
		return false, errors.Wrap(err, "could not create a client for the destination cluster")
	}

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
		return false, errors.Wrap(err, "failed to create namespace in destination cluster")
	default:
		logger.Info("namespace created")
	}

	// Check for the ClusterDeployment on the destination cluster.
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
	cdKey, err := client.ObjectKeyFromObject(cd)
	if err != nil {
		logger.WithError(err).Error("could not get object key for clusterdeployment")
		return false, errors.Wrap(err, "could not get object key for clusterdeployment")
	}
	destCD := &hivev1.ClusterDeployment{}
	switch err := destClient.Get(context.Background(), cdKey, destCD); {
	case apierrors.IsNotFound(err):
		logger.Info("clusterdeployment absent in destination cluster")
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get clusterdeployment in destination cluster")
		return false, errors.Wrap(err, "failed to get clusterdeployment in destination cluster")
	default:
		logger.Info("clusterdeployment already exists in destination cluster")
		if destCD.Spec.BaseDomain == cd.Spec.BaseDomain {
			// The ClusterDeployment must be the last resource copied. If the ClusterDeployment exists on the
			// destination cluster, then the copy is complete.
			return true, nil
		}
		logger.Warn("clusterdeployment in destination cluster does not match the one being relocated")
		if conds, changed := controllerutils.SetClusterDeploymentConditionWithChangeCheck(
			cd.Status.Conditions,
			hivev1.RelocationFailedCondition,
			corev1.ConditionTrue,
			"ClusterDeploymentMismatch",
			"The ClusterDeployment in the destination cluster does not match the one being relocated. To relocate this ClusterDeployment, the ClusterDeployment in the destination cluster first must be deleted.",
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		); changed {
			cd.Status.Conditions = conds
			if err := r.Status().Update(context.Background(), cd); err != nil {
				logger.WithError(err).Log(controllerutils.LogLevel(err), "could not update condition")
				return false, errors.Wrap(err, "could not update condition")
			}
		}
		return false, nil
	}

	// copy dependent resources
	for _, t := range typesToCopy() {
		if err := r.copyResources(cd, destClient, t, logger); err != nil {
			return false, errors.Wrapf(err, "failed to copy %T", t)
		}
	}

	// copy dnszone
	dnsZone := &hivev1.DNSZone{}
	switch err := r.Get(context.Background(), client.ObjectKey{Namespace: cd.Namespace, Name: controllerutils.DNSZoneName(cd.Name)}, dnsZone); {
	case apierrors.IsNotFound(err):
		logger.Debug("no DNSZone to copy")
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to get dnszone in source cluster")
		return false, errors.Wrap(err, "failed to get dnszone in source cluster")
	default:
		if err := r.copyResource(dnsZone, destClient, false, logger); err != nil {
			return false, errors.Wrap(err, "failed to copy dnszone")
		}
	}

	// copy clusterdeployment
	if err := r.copyResource(cd, destClient, true, logger); err != nil {
		return false, errors.Wrap(err, "failed to copy clusterdeployment")
	}

	metricSuccessfulClusterRelocations.WithLabelValues(relocator.Name).Inc()

	return true, nil
}

func (r *ReconcileClusterRelocate) copyResources(cd *hivev1.ClusterDeployment, destClient client.Client, objectList runtime.Object, logger log.FieldLogger) error {
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
	objToCreate := obj.DeepCopyObject()
	switch err := destClient.Create(context.Background(), objToCreate); {
	case err == nil:
		logger.Info("resource created in destination cluster")
	case apierrors.IsAlreadyExists(err):
		if failIfExists {
			return errors.Wrap(err, "resource already exists in destination cluster")
		}
		logger.Info("resource already exists in destination cluster; deleting and copying anew")
		if err := r.replaceResourceIfChanged(destClient, obj, logger); err != nil {
			return errors.Wrap(err, "failed to sync existing resource")
		}
	default:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not create resource in destination cluster")
		return errors.Wrap(err, "could not create resource in destination cluster")
	}
	return nil
}

func (r *ReconcileClusterRelocate) replaceResourceIfChanged(destClient client.Client, srcObj runtime.Object, logger log.FieldLogger) error {
	switch srcObj.(type) {
	case *hivev1.ClusterDeployment:
		logger.Error("attempting to replace a ClusterDeployment")
		return errors.New("resource already exists in destination cluster")
	}
	objKey, err := client.ObjectKeyFromObject(srcObj)
	if err != nil {
		logger.WithError(err).Error("could not get object key")
		return errors.Wrap(err, "could not get object key")
	}
	destObj := reflect.New(reflect.TypeOf(srcObj).Elem()).Interface().(runtime.Object)
	if err := destClient.Get(context.Background(), objKey, destObj); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not get resource from destination cluster")
		return errors.Wrap(err, "could not get resource from destination cluster")
	}
	clearedDestObj, err := prepareForComparison(destObj)
	if err != nil {
		logger.WithError(err).Error("could not clear fields of destination resource")
		return errors.Wrap(err, "could not clear fields of destination resource")
	}
	if reflect.DeepEqual(srcObj, clearedDestObj) {
		// Do nothing. Resource in destination cluster is in sync.
		return nil
	} else {
		if logger.WithFields(nil).Logger.IsLevelEnabled(log.DebugLevel) {
			logger.WithField("diff", diff.ObjectReflectDiff(srcObj, clearedDestObj)).
				Debug("resource in destination cluster is out of sync")
		}
	}
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

	objMeta, err := meta.Accessor(obj)
	if err != nil {
		return nil, errors.Wrap(err, "could not get object meta")
	}
	if err := copyInstanceSpecificMeta(objMeta, &metav1.ObjectMeta{}); err != nil {
		return nil, errors.Wrap(err, "could not clear instance-specific meta")
	}

	switch t := obj.(type) {
	case *corev1.Secret:
		// Do nothing
	case *corev1.ConfigMap:
		// Do nothing
	case *hivev1.MachinePool:
		t.Status = hivev1.MachinePoolStatus{}
	case *hivev1.SyncSet:
		t.Status = hivev1.SyncSetStatus{}
	case *hivev1.SyncIdentityProvider:
		t.Status = hivev1.IdentityProviderStatus{}
	case *hivev1.DNSZone:
		t.Status = hivev1.DNSZoneStatus{}
	case *hivev1.ClusterDeployment:
		t.Status = hivev1.ClusterDeploymentStatus{}
		delete(t.Annotations, constants.RelocatingAnnotation)
	default:
		return nil, errors.Errorf("unknown type to relocate: %T", t)
	}

	return obj, nil
}

func copyInstanceSpecificMeta(to, from metav1.Object) error {
	to.SetSelfLink(from.GetSelfLink())
	to.SetUID(from.GetUID())
	to.SetResourceVersion(from.GetResourceVersion())
	to.SetGeneration(from.GetGeneration())
	to.SetCreationTimestamp(from.GetCreationTimestamp())
	to.SetOwnerReferences(from.GetOwnerReferences())
	to.SetClusterName(from.GetClusterName())
	return nil
}

func (r *ReconcileClusterRelocate) ignoreResource(obj runtime.Object, logger log.FieldLogger) (bool, error) {
	switch t := obj.(type) {
	case *corev1.Secret:
		return r.ignoreSecret(t, logger)
	default:
		return false, nil
	}
}

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

func recordMetricForAbortedRelocate(abortedRelocate, abortedReason string) {
	if abortedReason == "" {
		return
	}
	metricAbortedClusterRelocations.WithLabelValues(abortedRelocate, abortedReason).Inc()
}

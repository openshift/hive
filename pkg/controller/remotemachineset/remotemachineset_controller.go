package remotemachineset

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"
	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	controllerName = "remotemachineset"

	machinePoolNameLabel = "hive.openshift.io/machine-pool"
	finalizer            = "hive.openshift.io/remotemachineset"
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &ReconcileRemoteMachineSet{
		Client: controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", controllerName),
	}
	r.actuatorBuilder = func(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) (Actuator, error) {
		return r.createActuator(cd, remoteMachineSets, logger)
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, controllerName)
	}

	// Create a new controller
	c, err := controller.New("remotemachineset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to MachinePools
	err = c.Watch(&source.Kind{Type: &hivev1.MachinePool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(r.clusterDeploymentWatchHandler),
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileRemoteMachineSet) clusterDeploymentWatchHandler(a handler.MapObject) []reconcile.Request {
	retval := []reconcile.Request{}

	cd := a.Object.(*hivev1.ClusterDeployment)
	if cd == nil {
		// Wasn't a clusterdeployment, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a.Object)
		return retval
	}

	pools := &hivev1.MachinePoolList{}
	err := r.List(context.TODO(), pools)
	if err != nil {
		// Could not list machine pools
		r.logger.Errorf("Error listing machine pools. Value: %+v", a.Object)
		return retval
	}

	for _, pool := range pools.Items {
		if pool.Spec.ClusterDeploymentRef.Name != cd.Name {
			continue
		}
		key := client.ObjectKey{Namespace: pool.Namespace, Name: pool.Name}
		retval = append(retval, reconcile.Request{NamespacedName: key})
	}

	return retval
}

var _ reconcile.Reconciler = &ReconcileRemoteMachineSet{}

// ReconcileRemoteMachineSet reconciles the MachineSets generated from a ClusterDeployment object
type ReconcileRemoteMachineSet struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder

	// actuatorBuilder is a function pointer to the function that builds the actuator
	actuatorBuilder func(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) (Actuator, error)
}

// Reconcile reads that state of the cluster for a MachinePool object and makes changes to the
// remote cluster MachineSets based on the state read
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	logger := r.logger.WithFields(log.Fields{
		"machinePool": request.Name,
		"namespace":   request.Namespace,
	})
	logger.Info("reconciling machine pool")

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		logger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the MachinePool instance
	pool := &hivev1.MachinePool{}
	if err := r.Get(context.TODO(), request.NamespacedName, pool); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		logger.WithError(err).Error("error looking up machine pool")
		return reconcile.Result{}, err
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		if pool.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
	}

	cd := &hivev1.ClusterDeployment{}
	switch err := r.Get(
		context.TODO(),
		client.ObjectKey{Namespace: pool.Namespace, Name: pool.Spec.ClusterDeploymentRef.Name},
		cd,
	); {
	case apierrors.IsNotFound(err):
		logger.Debug("clusterdeployment does not exist")
		return r.removeFinalizer(pool, logger)
	case err != nil:
		logger.WithError(err).Error("error looking up cluster deploymnet")
		return reconcile.Result{}, err
	}

	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		logger.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
		return reconcile.Result{}, nil
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return r.removeFinalizer(pool, logger)
	}

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		logger.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		logger.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		controllerutils.AddFinalizer(pool, finalizer)
		err := r.Update(context.Background(), pool)
		if err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "could not add finalizer")
		}
		return reconcile.Result{}, err
	}

	remoteClientBuilder := r.remoteClusterAPIClientBuilder(cd)

	// If the cluster is unreachable, do not reconcile.
	if remoteClientBuilder.Unreachable() {
		logger.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	remoteClusterAPIClient, err := remoteClientBuilder.Build()
	if err != nil {
		logger.WithError(err).Error("error building remote cluster-api client connection")
		return reconcile.Result{}, err
	}

	logger.Info("reconciling machine pool for cluster deployment")

	remoteMachineSets, err := r.getRemoteMachineSets(cd, remoteClusterAPIClient, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	generatedMachineSets, err := r.generateMachineSets(pool, cd, remoteMachineSets, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	switch result, err := r.ensureEnoughReplicas(pool, generatedMachineSets, logger); {
	case err != nil:
		return reconcile.Result{}, err
	case result != nil:
		return *result, nil
	}

	machineSets, err := r.syncMachineSets(pool, cd, generatedMachineSets, remoteMachineSets, remoteClusterAPIClient, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := r.syncMachineAutoscalers(pool, cd, machineSets, remoteClusterAPIClient, logger); err != nil {
		return reconcile.Result{}, err
	}

	if err := r.syncClusterAutoscaler(pool, cd, remoteClusterAPIClient, logger); err != nil {
		return reconcile.Result{}, err
	}

	if pool.DeletionTimestamp != nil {
		return r.removeFinalizer(pool, logger)
	}

	return reconcile.Result{}, r.updatePoolStatusForMachineSets(pool, machineSets, logger)
}

func (r *ReconcileRemoteMachineSet) getRemoteMachineSets(
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) (*machineapi.MachineSetList, error) {
	remoteMachineSets := &machineapi.MachineSetList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
	if err := remoteClusterAPIClient.List(
		context.Background(),
		remoteMachineSets,
		client.UseListOptions(&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}}),
	); err != nil {
		logger.WithError(err).Error("unable to fetch remote machine sets")
		return nil, err
	}
	logger.Infof("found %v remote machine sets", len(remoteMachineSets.Items))
	return remoteMachineSets, nil
}

func (r *ReconcileRemoteMachineSet) generateMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	remoteMachineSets *machineapi.MachineSetList,
	logger log.FieldLogger,
) ([]*machineapi.MachineSet, error) {
	if pool.DeletionTimestamp != nil {
		return nil, nil
	}

	actuator, err := r.actuatorBuilder(cd, remoteMachineSets.Items, logger)
	if err != nil {
		logger.WithError(err).Error("unable to create actuator")
		return nil, err
	}

	// Generate expected MachineSets for Platform from InstallConfig
	generatedMachineSets, err := actuator.GenerateMachineSets(cd, pool, logger)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate machinesets")
	}

	for i, ms := range generatedMachineSets {
		if pool.Spec.Autoscaling != nil {
			min, _ := getMinMaxReplicasForMachineSet(pool, generatedMachineSets, i)
			ms.Spec.Replicas = &min
		}

		if ms.Labels == nil {
			ms.Labels = map[string]string{}
		}
		ms.Labels[machinePoolNameLabel] = pool.Spec.Name

		// Apply hive MachinePool labels to MachineSet MachineSpec.
		ms.Spec.Template.Spec.ObjectMeta.Labels = make(map[string]string, len(pool.Spec.Labels))
		for key, value := range pool.Spec.Labels {
			ms.Spec.Template.Spec.ObjectMeta.Labels[key] = value
		}

		// Apply hive MachinePool taints to MachineSet MachineSpec.
		ms.Spec.Template.Spec.Taints = pool.Spec.Taints
	}

	logger.Infof("generated %v worker machine sets", len(generatedMachineSets))

	return generatedMachineSets, nil
}

// ensureEnoughReplicas ensures that the min replicas in the machine pool is
// large enough to cover all of the zones for the machine pool. When using
// auto-scaling, every machineset needs to have a minimum replicas of 1.
// If the reconcile.Result returned in non-nil, then the reconciliation loop
// should stop, returning that result. This is used to prevent re-queueing
// a machine pool that will always fail with not enough replicas. There is
// nothing that the controller can do with such a machine pool until the user
// makes updates to the machine pool.
func (r *ReconcileRemoteMachineSet) ensureEnoughReplicas(
	pool *hivev1.MachinePool,
	generatedMachineSets []*machineapi.MachineSet,
	logger log.FieldLogger,
) (*reconcile.Result, error) {
	if pool.Spec.Autoscaling == nil {
		return nil, nil
	}
	if pool.Spec.Autoscaling.MinReplicas < int32(len(generatedMachineSets)) {
		logger.WithField("machinesets", len(generatedMachineSets)).
			WithField("minReplicas", pool.Spec.Autoscaling.MinReplicas).
			Warning("when auto-scaling, the MachinePool must have at least one replica for each MachineSet")
		conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
			pool.Status.Conditions,
			hivev1.NotEnoughReplicasMachinePoolCondition,
			corev1.ConditionTrue,
			"MinReplicasTooSmall",
			fmt.Sprintf("When auto-scaling, the MachinePool must have at least one replica for each MachineSet. The minReplicas must be at least %d", len(generatedMachineSets)),
			controllerutils.UpdateConditionIfReasonOrMessageChange,
		)
		if changed {
			pool.Status.Conditions = conds
			if err := r.Status().Update(context.Background(), pool); err != nil {
				logger.WithError(err).Error("failed to update MachinePool conditions")
				return &reconcile.Result{}, err
			}
		}
		return &reconcile.Result{}, nil
	}
	conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		hivev1.NotEnoughReplicasMachinePoolCondition,
		corev1.ConditionFalse,
		"EnoughReplicas",
		"The MachinePool has at least one replica for each MachineSet",
		controllerutils.UpdateConditionNever,
	)
	if changed {
		pool.Status.Conditions = conds
		err := r.Status().Update(context.Background(), pool)
		if err != nil {
			logger.WithError(err).Error("failed to update MachinePool conditions")
		}
		return &reconcile.Result{}, err
	}
	return nil, nil
}

func (r *ReconcileRemoteMachineSet) syncMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	generatedMachineSets []*machineapi.MachineSet,
	remoteMachineSets *machineapi.MachineSetList,
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) ([]*machineapi.MachineSet, error) {
	result := make([]*machineapi.MachineSet, len(generatedMachineSets))

	machineSetsToDelete := []*machineapi.MachineSet{}
	machineSetsToCreate := []*machineapi.MachineSet{}
	machineSetsToUpdate := []*machineapi.MachineSet{}

	// Find MachineSets that need updating/creating
	for i, ms := range generatedMachineSets {
		found := false
		for _, rMS := range remoteMachineSets.Items {
			if ms.Name == rMS.Name {
				found = true
				objectModified := false
				objectMetaModified := false
				resourcemerge.EnsureObjectMeta(&objectMetaModified, &rMS.ObjectMeta, ms.ObjectMeta)
				msLog := logger.WithField("machineset", rMS.Name)

				if pool.Spec.Autoscaling == nil {
					if *rMS.Spec.Replicas != *ms.Spec.Replicas {
						msLog.WithFields(log.Fields{
							"desired":  *ms.Spec.Replicas,
							"observed": *rMS.Spec.Replicas,
						}).Info("replicas out of sync")
						rMS.Spec.Replicas = ms.Spec.Replicas
						objectModified = true
					}
				} else {
					// If minReplicas==maxReplicas, then the autoscaler will ignore the machineset,
					// even if the replicas in the machineset is not equal to the min and max.
					// To ensure that the replicas falls within min and max regardless, Hive needs
					// to set the replicas to explicitly be within the desired range.
					min, max := getMinMaxReplicasForMachineSet(pool, generatedMachineSets, i)
					switch {
					case rMS.Spec.Replicas == nil:
						msLog.WithField("observed", nil).WithField("min", min).WithField("max", max).Info("setting replicas to min")
						rMS.Spec.Replicas = &min
						objectModified = true
					case *rMS.Spec.Replicas < min:
						msLog.WithField("observed", *rMS.Spec.Replicas).WithField("min", min).WithField("max", max).Info("setting replicas to min")
						rMS.Spec.Replicas = &min
						objectModified = true
					case *rMS.Spec.Replicas > max:
						msLog.WithField("observed", *rMS.Spec.Replicas).WithField("min", min).WithField("max", max).Info("setting replicas to max")
						rMS.Spec.Replicas = &max
						objectModified = true
					default:
						msLog.WithField("observed", *rMS.Spec.Replicas).WithField("min", min).WithField("max", max).Debug("replicas within range")
					}
				}

				// Update if the labels on the remote machineset are different than the labels on the generated machineset.
				// If the length of both labels is zero, then they match, even if one is a nil map and the other is an empty map.
				if rl, l := rMS.Spec.Template.Spec.Labels, ms.Spec.Template.Spec.Labels; (len(rl) != 0 || len(l) != 0) && !reflect.DeepEqual(rl, l) {
					msLog.WithField("desired", l).WithField("observed", rl).Info("labels out of sync")
					rMS.Spec.Template.Spec.Labels = l
					objectModified = true
				}

				// Update if the taints on the remote machineset are different than the taints on the generated machineset.
				// If the length of both taints is zero, then they match, even if one is a nil slice and the other is an empty slice.
				if rt, t := rMS.Spec.Template.Spec.Taints, ms.Spec.Template.Spec.Taints; (len(rt) != 0 || len(t) != 0) && !reflect.DeepEqual(rt, t) {
					msLog.WithField("desired", t).WithField("observed", rt).Info("taints out of sync")
					rMS.Spec.Template.Spec.Taints = t
					objectModified = true
				}

				if objectMetaModified || objectModified {
					rMS.Generation++
					machineSetsToUpdate = append(machineSetsToUpdate, &rMS)
				}

				result[i] = &rMS
				break
			}
		}

		if !found {
			machineSetsToCreate = append(machineSetsToCreate, ms)
			result[i] = ms
		}
	}

	// Find MachineSets that need deleting
	for i, rMS := range remoteMachineSets.Items {
		if !isControlledByMachinePool(cd, pool, &rMS) {
			continue
		}
		delete := true
		if pool.DeletionTimestamp == nil {
			for _, ms := range generatedMachineSets {
				if rMS.Name == ms.Name {
					delete = false
					break
				}
			}
		}
		if delete {
			machineSetsToDelete = append(machineSetsToDelete, &remoteMachineSets.Items[i])
		}
	}

	for _, ms := range machineSetsToCreate {
		logger.WithField("machineset", ms.Name).Info("creating machineset")
		if err := remoteClusterAPIClient.Create(context.Background(), ms); err != nil {
			logger.WithError(err).Error("unable to create machine set")
			return nil, err
		}
	}

	for _, ms := range machineSetsToUpdate {
		logger.WithField("machineset", ms.Name).Info("updating machineset")
		if err := remoteClusterAPIClient.Update(context.Background(), ms); err != nil {
			logger.WithError(err).Error("unable to update machine set")
			return nil, err
		}
	}

	for _, ms := range machineSetsToDelete {
		logger.WithField("machineset", ms.Name).Info("deleting machineset")
		if err := remoteClusterAPIClient.Delete(context.Background(), ms); err != nil {
			logger.WithError(err).Error("unable to delete machine set")
			return nil, err
		}
	}

	logger.Info("done reconciling machine sets for machine pool")
	return result, nil
}

func (r *ReconcileRemoteMachineSet) syncMachineAutoscalers(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	machineSets []*machineapi.MachineSet,
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) error {
	// List MachineAutoscalers from remote cluster
	remoteMachineAutoscalers := &autoscalingv1beta1.MachineAutoscalerList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(autoscalingv1beta1.SchemeGroupVersion.WithKind("MachineAutoscaler"))
	if err := remoteClusterAPIClient.List(
		context.Background(),
		remoteMachineAutoscalers,
		client.UseListOptions(&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}}),
	); err != nil {
		logger.WithError(err).Error("unable to fetch remote machine autoscalers")
		return err
	}
	logger.Infof("found %v remote machine autoscalers", len(remoteMachineAutoscalers.Items))

	machineAutoscalersToDelete := []*autoscalingv1beta1.MachineAutoscaler{}
	machineAutoscalersToCreate := []*autoscalingv1beta1.MachineAutoscaler{}
	machineAutoscalersToUpdate := []*autoscalingv1beta1.MachineAutoscaler{}

	if pool.DeletionTimestamp == nil && pool.Spec.Autoscaling != nil {
		// Find MachineAutoscalers that need updating/creating
		for i, ms := range machineSets {
			minReplicas, maxReplicas := getMinMaxReplicasForMachineSet(pool, machineSets, i)
			found := false
			for _, rMA := range remoteMachineAutoscalers.Items {
				if ms.Name == rMA.Name {
					found = true
					objectModified := false
					maLog := logger.WithField("machineautoscaler", rMA.Name)

					if rMA.Spec.MinReplicas != minReplicas {
						maLog.WithField("desired", minReplicas).
							WithField("observed", rMA.Spec.MinReplicas).
							Info("min replicas out of sync")
						rMA.Spec.MinReplicas = minReplicas
						objectModified = true
					}

					if rMA.Spec.MaxReplicas != maxReplicas {
						maLog.WithField("desired", maxReplicas).
							WithField("observed", rMA.Spec.MaxReplicas).
							Info("max replicas out of sync")
						rMA.Spec.MaxReplicas = maxReplicas
						objectModified = true
					}

					if objectModified {
						machineAutoscalersToUpdate = append(machineAutoscalersToUpdate, &rMA)
					}
					break
				}
			}

			if !found {
				ma := &autoscalingv1beta1.MachineAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: ms.Namespace,
						Name:      ms.Name,
						Labels: map[string]string{
							machinePoolNameLabel: pool.Spec.Name,
						},
					},
					Spec: autoscalingv1beta1.MachineAutoscalerSpec{
						MinReplicas: minReplicas,
						MaxReplicas: maxReplicas,
						ScaleTargetRef: autoscalingv1beta1.CrossVersionObjectReference{
							APIVersion: ms.APIVersion,
							Kind:       ms.Kind,
							Name:       ms.Name,
						},
					},
				}
				machineAutoscalersToCreate = append(machineAutoscalersToCreate, ma)
			}

		}
	}

	// Find MachineAutoscalers that need deleting
	for i, rMA := range remoteMachineAutoscalers.Items {
		if !isControlledByMachinePool(cd, pool, &rMA) {
			continue
		}
		delete := true
		if pool.DeletionTimestamp == nil && pool.Spec.Autoscaling != nil {
			for _, ms := range machineSets {
				if rMA.Name == ms.Name {
					delete = false
					break
				}
			}
		}
		if delete {
			machineAutoscalersToDelete = append(machineAutoscalersToDelete, &remoteMachineAutoscalers.Items[i])
		}
	}

	for _, ma := range machineAutoscalersToCreate {
		logger.WithField("machineautoscaler", ma.Name).Info("creating machineautoscaler")
		if err := remoteClusterAPIClient.Create(context.Background(), ma); err != nil {
			logger.WithError(err).Error("unable to create machine autoscaler")
			return err
		}
	}

	for _, ma := range machineAutoscalersToUpdate {
		logger.WithField("machineautoscaler", ma.Name).Info("updating machineautoscaler")
		if err := remoteClusterAPIClient.Update(context.Background(), ma); err != nil {
			logger.WithError(err).Error("unable to update machine autoscaler")
			return err
		}
	}

	for _, ma := range machineAutoscalersToDelete {
		logger.WithField("machineautoscaler", ma.Name).Info("deleting machineautoscaler")
		if err := remoteClusterAPIClient.Delete(context.Background(), ma); err != nil {
			logger.WithError(err).Error("unable to delete machine autoscaler")
			return err
		}
	}

	logger.Info("done reconciling machine autoscalers for cluster deployment")
	return nil
}

func (r *ReconcileRemoteMachineSet) syncClusterAutoscaler(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) error {
	if pool.Spec.Autoscaling == nil {
		return nil
	}
	remoteClusterAutoscalers := &autoscalingv1.ClusterAutoscalerList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(autoscalingv1.SchemeGroupVersion.WithKind("ClusterAutoscaler"))
	if err := remoteClusterAPIClient.List(
		context.Background(),
		remoteClusterAutoscalers,
		client.UseListOptions(&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}}),
	); err != nil {
		logger.WithError(err).Error("unable to fetch remote cluster autoscalers")
		return err
	}
	logger.Infof("found %v remote cluster autoscalers", len(remoteClusterAutoscalers.Items))
	var defaultClusterAutoscaler *autoscalingv1.ClusterAutoscaler
	for i, rCA := range remoteClusterAutoscalers.Items {
		if rCA.Name == "default" {
			defaultClusterAutoscaler = &remoteClusterAutoscalers.Items[i]
			break
		}
	}
	if defaultClusterAutoscaler != nil {
		if spec := &defaultClusterAutoscaler.Spec; spec.ScaleDown == nil || !spec.ScaleDown.Enabled {
			logger.Info("updaing cluster autoscaler")
			if spec.ScaleDown == nil {
				spec.ScaleDown = &autoscalingv1.ScaleDownConfig{}
			}
			spec.ScaleDown.Enabled = true
			if err := remoteClusterAPIClient.Update(context.Background(), defaultClusterAutoscaler); err != nil {
				logger.WithError(err).Error("could not update cluster autoscaler")
				return err
			}
		}
	} else {
		logger.Info("creating cluster autoscaler")
		defaultClusterAutoscaler = &autoscalingv1.ClusterAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: autoscalingv1.ClusterAutoscalerSpec{
				ScaleDown: &autoscalingv1.ScaleDownConfig{
					Enabled: true,
				},
			},
		}
		if err := remoteClusterAPIClient.Create(context.Background(), defaultClusterAutoscaler); err != nil {
			logger.WithError(err).Error("could not create cluster autoscaler")
			return err
		}
	}
	return nil
}

func (r *ReconcileRemoteMachineSet) updatePoolStatusForMachineSets(
	pool *hivev1.MachinePool,
	machineSets []*machineapi.MachineSet,
	logger log.FieldLogger,
) error {
	origPool := pool.DeepCopy()

	pool.Status.MachineSets = make([]hivev1.MachineSetStatus, len(machineSets))
	pool.Status.Replicas = 0
	for i, ms := range machineSets {
		var min, max int32
		if pool.Spec.Autoscaling == nil {
			min = *ms.Spec.Replicas
			max = *ms.Spec.Replicas
		} else {
			min, max = getMinMaxReplicasForMachineSet(pool, machineSets, i)
		}
		pool.Status.MachineSets[i] = hivev1.MachineSetStatus{
			Name:        ms.Name,
			Replicas:    *ms.Spec.Replicas,
			MinReplicas: min,
			MaxReplicas: max,
		}
		pool.Status.Replicas += *ms.Spec.Replicas
	}

	if (len(origPool.Status.MachineSets) == 0 && len(pool.Status.MachineSets) == 0) ||
		reflect.DeepEqual(origPool.Status, pool.Status) {
		return nil
	}

	return errors.Wrap(r.Status().Update(context.Background(), pool), "failed to update pool status")
}

func (r *ReconcileRemoteMachineSet) createActuator(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) (Actuator, error) {
	switch {
	case cd.Spec.Platform.AWS != nil:
		creds := &corev1.Secret{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      cd.Spec.Platform.AWS.CredentialsSecretRef.Name,
				Namespace: cd.Namespace,
			},
			creds,
		); err != nil {
			return nil, err
		}
		return NewAWSActuator(creds, cd.Spec.Platform.AWS.Region, remoteMachineSets, r.scheme, logger)
	case cd.Spec.Platform.GCP != nil:
		creds := &corev1.Secret{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      cd.Spec.Platform.GCP.CredentialsSecretRef.Name,
				Namespace: cd.Namespace,
			},
			creds,
		); err != nil {
			return nil, err
		}
		return NewGCPActuator(creds, logger)
	default:
		return nil, errors.New("unsupported platform")
	}
}

func baseMachinePool(pool *hivev1.MachinePool) *installertypes.MachinePool {
	return &installertypes.MachinePool{
		Name:     pool.Spec.Name,
		Replicas: pool.Spec.Replicas,
	}
}

func isControlledByMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, obj metav1.Object) bool {
	prefix := strings.Join([]string{cd.Spec.ClusterName, pool.Spec.Name, ""}, "-")
	return strings.HasPrefix(obj.GetName(), prefix) ||
		obj.GetLabels()[machinePoolNameLabel] == pool.Spec.Name
}

func (r *ReconcileRemoteMachineSet) removeFinalizer(pool *hivev1.MachinePool, logger log.FieldLogger) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return reconcile.Result{}, nil
	}
	controllerutils.DeleteFinalizer(pool, finalizer)
	err := r.Update(context.Background(), pool)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer")
	}
	return reconcile.Result{}, err
}

func getMinMaxReplicasForMachineSet(pool *hivev1.MachinePool, machineSets []*machineapi.MachineSet, machineSetIndex int) (min, max int32) {
	noOfMachineSets := int32(len(machineSets))
	min = pool.Spec.Autoscaling.MinReplicas / noOfMachineSets
	if int32(machineSetIndex) < pool.Spec.Autoscaling.MinReplicas%noOfMachineSets {
		min++
	}
	max = pool.Spec.Autoscaling.MaxReplicas / noOfMachineSets
	if int32(machineSetIndex) < pool.Spec.Autoscaling.MaxReplicas%noOfMachineSets {
		max++
	}
	if max < min {
		max = min
	}
	return
}

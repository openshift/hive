package machinepool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	machineapi "github.com/openshift/api/machine/v1beta1"
	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"
	cpms "github.com/openshift/cluster-control-plane-machine-set-operator/pkg/machineproviders/providers/openshift/machine/v1beta1/providerconfig"
	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
	"github.com/openshift/hive/pkg/util/logrus"
	"github.com/openshift/hive/pkg/util/scheme"
)

const (
	ControllerName              = hivev1.MachinePoolControllerName
	machinePoolNameLabel        = "hive.openshift.io/machine-pool"
	finalizer                   = "hive.openshift.io/remotemachineset"
	masterMachineLabelSelector  = "machine.openshift.io/cluster-api-machine-type=master"
	defaultPollInterval         = 30 * time.Minute
	defaultErrorRequeueInterval = 1 * time.Minute
	stsName                     = hivev1.DeploymentNameMachinepool
	capiClusterKey              = "machine.openshift.io/cluster-api-cluster"
	capiMachineSetKey           = "machine.openshift.io/cluster-api-machineset"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("MachinePool")

	// machinePoolConditions are the Machine Pool conditions controlled by machinepool controller
	machinePoolConditions = []hivev1.MachinePoolConditionType{
		hivev1.NotEnoughReplicasMachinePoolCondition,
		hivev1.NoMachinePoolNameLeasesAvailable,
		hivev1.InvalidSubnetsMachinePoolCondition,
		hivev1.UnsupportedConfigurationMachinePoolCondition,
		hivev1.MachineSetsGeneratedMachinePoolCondition,
		hivev1.SyncedMachinePoolCondition,
	}
)

// Add creates a new MachinePool Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)

	scheme := scheme.GetScheme()

	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}

	ordinalID, err := controllerutils.GetMyOrdinalID(ControllerName, logger)
	if err != nil {
		return err
	}

	pollInterval := defaultPollInterval
	if envPollInterval := os.Getenv(constants.MachinePoolPollIntervalEnvVar); len(envPollInterval) > 0 {
		var err error
		pollInterval, err = time.ParseDuration(envPollInterval)
		if err != nil {
			log.WithError(err).WithField("reapplyInterval", envPollInterval).Errorf("unable to parse %s", constants.MachinePoolPollIntervalEnvVar)
			return err
		}
	}
	logger.Infof("using poll interval of %s", pollInterval)

	r := &ReconcileMachinePool{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &clientRateLimiter),
		scheme:       scheme,
		logger:       logger,
		expectations: controllerutils.NewExpectations(logger),
		ordinalID:    ordinalID,
		pollInterval: pollInterval,
	}
	r.actuatorBuilder = func(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, masterMachine *machineapi.Machine, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) (Actuator, error) {
		return r.createActuator(cd, pool, masterMachine, remoteMachineSets, logger)
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}

	// Create a new controller
	c, err := controller.New("machinepool-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             queueRateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to MachinePools
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.MachinePool{}, controllerutils.NewTypedRateLimitedUpdateEventHandler(&handler.TypedEnqueueRequestForObject[*hivev1.MachinePool]{}, IsErrorUpdateEvent)))
	if err != nil {
		return err
	}

	// Watch for MachinePoolNameLeases created for some MachinePools (currently just GCP):
	if err := r.watchMachinePoolNameLeases(mgr, c); err != nil {
		return errors.Wrap(err, "could not watch MachinePoolNameLeases")
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}, controllerutils.NewTypedRateLimitedUpdateEventHandler(
		handler.TypedEnqueueRequestsFromMapFunc(r.clusterDeploymentWatchHandler),
		controllerutils.IsClusterDeploymentErrorUpdateEvent)))
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileMachinePool) clusterDeploymentWatchHandler(ctx context.Context, cd *hivev1.ClusterDeployment) []reconcile.Request {
	retval := []reconcile.Request{}

	pools := &hivev1.MachinePoolList{}
	err := r.List(context.TODO(), pools)
	if err != nil {
		// Could not list machine pools
		r.logger.Errorf("Error listing machine pools. Value: %+v", cd)
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

var _ reconcile.Reconciler = &ReconcileMachinePool{}

// ReconcileMachinePool reconciles the MachineSets generated from a MachinePool object
type ReconcileMachinePool struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that gets a builder for building a client
	// for the remote cluster's API server
	remoteClusterAPIClientBuilder func(cd *hivev1.ClusterDeployment) remoteclient.Builder

	// actuatorBuilder is a function pointer to the function that builds the actuator
	actuatorBuilder func(
		cd *hivev1.ClusterDeployment,
		pool *hivev1.MachinePool,
		masterMachine *machineapi.Machine,
		remoteMachineSets []machineapi.MachineSet,
		logger log.FieldLogger,
	) (Actuator, error)

	// A TTLCache of machinepoolnamelease creates each machinepool expects to see. Note that not all actuators make use
	// of expectations.
	expectations controllerutils.ExpectationsInterface

	// Which hive-machinepool StatefulSet replica this reconciler instance is running in.
	ordinalID int64

	// pollInterval is the maximum time we'll wait before re-reconciling a given MachinePool.
	// Zero to disable (re-reconcile will be prompted by the usual things, e.g. CR updates).
	pollInterval time.Duration
}

// Reconcile reads that state of the cluster for a MachinePool object and makes changes to the
// remote cluster MachineSets based on the state read
func (r *ReconcileMachinePool) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := controllerutils.BuildControllerLogger(ControllerName, "machinePool", request.NamespacedName)
	logger.Info("reconciling machine pool")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, logger)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the MachinePool instance
	pool := &hivev1.MachinePool{}
	if err := r.Get(context.TODO(), request.NamespacedName, pool); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return
			r.logger.Debug("object no longer exists")
			r.expectations.DeleteExpectations(request.String())
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		logger.WithError(err).Error("error looking up machine pool")
		return reconcile.Result{}, err
	}
	// NOTE: This may be sparse if we haven't yet synced from the ClusterDeployment (below)
	logger = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: pool}, logger)

	me, err := controllerutils.IsUIDAssignedToMe(r.Client, stsName, pool.UID, r.ordinalID, logger)
	if !me || err != nil {
		if err != nil {
			logger.WithError(err).Error("failed determining which instance is assigned to sync this MachinePool")
			return reconcile.Result{}, err
		}

		logger.Debug("not syncing because this MachinePool is not assigned to me")
		recobsrv.SetOutcome(hivemetrics.ReconcileOutcomeSkippedSync)
		return reconcile.Result{}, nil
	}

	// Initialize machine pool conditions if not present
	newConditions, changed := controllerutils.InitializeMachinePoolConditions(pool.Status.Conditions, machinePoolConditions)
	if pool.Status.ControlledByReplica == nil || r.ordinalID != *pool.Status.ControlledByReplica {
		pool.Status.ControlledByReplica = &r.ordinalID
		changed = true
	}
	if changed {
		pool.Status.Conditions = newConditions
		logger.Info("initializing remote machineset controller conditions")
		if err := r.Status().Update(context.TODO(), pool); err != nil {
			logger.WithError(err).Log(controllerutils.LogLevel(err), "failed to update machine pool status")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		if pool.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
	}

	if !r.expectations.SatisfiedExpectations(request.String()) {
		logger.Debug("waiting for expectations to be satisfied")
		return reconcile.Result{}, nil
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
	// Sync log annotations from the CD to the pool, if necessary.
	if controllerutils.CopyLogAnnotation(cd, pool) {
		return reconcile.Result{}, r.Update(context.Background(), pool)
	}

	if controllerutils.IsClusterPausedOrRelocating(cd, logger) {
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

	// Default "success" for fake clusters
	ret, err := reconcile.Result{}, nil
	if controllerutils.IsFakeCluster(cd) {
		logger.Info("skipping reconcile for fake cluster")
		if pool.DeletionTimestamp != nil {
			return r.removeFinalizer(pool, logger)
		}
	} else {
		// Real clusters
		ret, err = r.reconcile(pool, cd, logger)
	}

	if err == nil && r.pollInterval > 0 && ret.RequeueAfter == 0 {
		ret.RequeueAfter = r.pollInterval + time.Duration(rand.Float64()*float64(time.Second))
		logger.WithField("RequeueAfter", ret.RequeueAfter).Info("requeueing with interval")
	}
	return ret, err
}

func (r *ReconcileMachinePool) reconcile(pool *hivev1.MachinePool, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (reconcile.Result, error) {
	remoteClusterAPIClient, unreachable, requeue := remoteclient.ConnectToRemoteCluster(
		cd,
		r.remoteClusterAPIClientBuilder(cd),
		r.Client,
		logger,
	)
	if unreachable {
		return reconcile.Result{Requeue: requeue}, nil
	}

	logger.Info("reconciling machine pool for cluster deployment")

	masterMachine, err := r.getMasterMachine(remoteClusterAPIClient, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	remoteMachineSets, err := r.getRemoteMachineSets(remoteClusterAPIClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not getRemoteMachineSets")
		return reconcile.Result{}, err
	}

	infrastructure, err := r.getInfrastructure(remoteClusterAPIClient, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	generatedMachineSets, generatedMachineLabels, proceed, err := r.generateMachineSets(pool, cd, masterMachine, remoteMachineSets, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not generateMachineSets")
		err := r.updateCondition(
			pool,
			hivev1.MachineSetsGeneratedMachinePoolCondition,
			corev1.ConditionFalse,
			"MachineSetGenerationFailed",
			// WARNING: Danger of thrashing. This is why we are using a static requeueAfter.
			err.Error(),
			logger,
		)
		// NOTE: err is nil unless condition update failed, in which case immediate requeue is appropriate.
		return reconcile.Result{RequeueAfter: defaultErrorRequeueInterval}, err
	}
	if err := r.updateCondition(
		pool,
		hivev1.MachineSetsGeneratedMachinePoolCondition,
		corev1.ConditionTrue,
		"MachineSetGenerationSucceeded",
		"MachineSets generated successfully",
		logger,
	); err != nil {
		return reconcile.Result{}, err
	}
	if !proceed {
		logger.Info("machineSets generator indicated not to proceed, returning")
		return reconcile.Result{}, nil
	}

	switch result, err := r.ensureEnoughReplicas(pool, generatedMachineSets, cd, logger); {
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not ensureEnoughReplicas")
		return reconcile.Result{}, err
	case result != nil:
		return *result, nil
	}

	machineSets, err := r.syncMachineSets(pool, cd, generatedMachineSets, remoteMachineSets, infrastructure, remoteClusterAPIClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncMachineSets")
		err := r.updateCondition(
			pool,
			hivev1.SyncedMachinePoolCondition,
			corev1.ConditionFalse,
			"MachineSetSyncFailed",
			// WARNING: Danger of thrashing. This is why we are using a static requeueAfter.
			err.Error(),
			logger,
		)
		// NOTE: err is nil unless condition update failed, in which case immediate requeue is appropriate.
		return reconcile.Result{RequeueAfter: defaultErrorRequeueInterval}, err
	}

	if err := r.syncMachineAutoscalers(pool, cd, machineSets, remoteClusterAPIClient, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncMachineAutoscalers")
		err := r.updateCondition(
			pool,
			hivev1.SyncedMachinePoolCondition,
			corev1.ConditionFalse,
			"MachineAutoscalerSyncFailed",
			// WARNING: Danger of thrashing. This is why we are using a static requeueAfter.
			err.Error(),
			logger,
		)
		// NOTE: err is nil unless condition update failed, in which case immediate requeue is appropriate.
		return reconcile.Result{RequeueAfter: defaultErrorRequeueInterval}, err
	}

	if err := r.syncClusterAutoscaler(pool, remoteClusterAPIClient, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncClusterAutoscaler")
		err := r.updateCondition(
			pool,
			hivev1.SyncedMachinePoolCondition,
			corev1.ConditionFalse,
			"ClusterAutoscalerSyncFailed",
			// WARNING: Danger of thrashing. This is why we are using a static requeueAfter.
			err.Error(),
			logger,
		)
		// NOTE: err is nil unless condition update failed, in which case immediate requeue is appropriate.
		return reconcile.Result{RequeueAfter: defaultErrorRequeueInterval}, err
	}
	if err := r.updateCondition(
		pool,
		hivev1.SyncedMachinePoolCondition,
		corev1.ConditionTrue,
		"SyncSucceeded",
		"Resources synced successfully",
		logger,
	); err != nil {
		return reconcile.Result{}, err
	}

	if pool.DeletionTimestamp != nil {
		return r.removeFinalizer(pool, logger)
	}

	return r.updatePoolStatusForMachineSets(pool, machineSets, generatedMachineLabels, remoteClusterAPIClient, logger)
}

func (r *ReconcileMachinePool) getInfrastructure(remoteClusterAPIClient client.Client, logger log.FieldLogger) (*configv1.Infrastructure, error) {
	infrastructure := &configv1.Infrastructure{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(configv1.SchemeGroupVersion.WithKind("Infrastructure"))
	if err := remoteClusterAPIClient.Get(
		context.Background(),
		types.NamespacedName{Name: "cluster"},
		infrastructure,
		// TODO: Why is this necessary?
		&client.GetOptions{Raw: &metav1.GetOptions{TypeMeta: tm}},
	); err != nil {
		logger.WithError(err).Error("unable to fetch infrastructure object")
		return nil, err
	}
	return infrastructure, nil
}

func (r *ReconcileMachinePool) getMasterMachine(
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) (*machineapi.Machine, error) {
	remoteMachines := &machineapi.MachineList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("Machine"))
	if err := remoteClusterAPIClient.List(
		context.Background(),
		remoteMachines,
		&client.ListOptions{
			Raw: &metav1.ListOptions{
				TypeMeta:      tm,
				LabelSelector: masterMachineLabelSelector,
			},
		},
	); err != nil {
		logger.WithError(err).Error("unable to fetch master machines")
		return nil, err
	}
	if len(remoteMachines.Items) == 0 {
		logger.Error("no master machines in cluster")
		return nil, errors.New("no master machines in cluster")
	}
	return &remoteMachines.Items[0], nil
}

func (r *ReconcileMachinePool) getRemoteMachineSets(
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) (*machineapi.MachineSetList, error) {
	remoteMachineSets := &machineapi.MachineSetList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
	if err := remoteClusterAPIClient.List(
		context.Background(),
		remoteMachineSets,
		&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}},
	); err != nil {
		logger.WithError(err).Error("unable to fetch remote machine sets")
		return nil, err
	}
	logger.Infof("found %v remote machine sets", len(remoteMachineSets.Items))
	return remoteMachineSets, nil
}

func (r *ReconcileMachinePool) generateMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	masterMachine *machineapi.Machine,
	remoteMachineSets *machineapi.MachineSetList,
	logger log.FieldLogger,
) ([]*machineapi.MachineSet, sets.Set[string], bool, error) {
	if pool.DeletionTimestamp != nil {
		return nil, nil, true, nil
	}

	actuator, err := r.actuatorBuilder(cd, pool, masterMachine, remoteMachineSets.Items, logger)
	if err != nil {
		logger.WithError(err).Error("unable to create actuator")
		return nil, nil, false, err
	}

	// Generate expected MachineSets for Platform from InstallConfig
	generatedMachineSets, proceed, err := actuator.GenerateMachineSets(cd, pool, logger)
	if err != nil {
		return nil, nil, false, errors.Wrap(err, "could not generate machinesets")
	} else if !proceed {
		logger.Info("actuator indicated not to proceed, returning")
		return nil, nil, false, nil
	}

	// HACK: Track generated machine labels so we don't add them to OwnedMachineLabels.
	generatedMachineLabels := sets.Set[string]{}

	for i, ms := range generatedMachineSets {
		if pool.Spec.Autoscaling != nil {
			min, _ := getMinMaxReplicasForMachineSet(pool, generatedMachineSets, i)
			ms.Spec.Replicas = &min
		}

		if ms.Labels == nil {
			ms.Labels = make(map[string]string, 2)
		}
		ms.Labels[machinePoolNameLabel] = pool.Spec.Name
		// Add the managed-by-Hive label:
		ms.Labels[constants.HiveManagedLabel] = "true"

		// Apply hive MachinePool labels to MachineSet MachineSpec.
		// (These end up on *Nodes*.)
		ms.Spec.Template.Spec.ObjectMeta.Labels = make(map[string]string, len(pool.Spec.Labels))
		for key, value := range pool.Spec.Labels {
			ms.Spec.Template.Spec.ObjectMeta.Labels[key] = value
		}

		// Apply hive MachinePool taints to MachineSet MachineSpec. Also collapse duplicates if present.
		ms.Spec.Template.Spec.Taints = *controllerutils.GetUniqueTaints(&pool.Spec.Taints)

		// Merge any user-specified labels to MachineSet MachineTemplateSpec.
		// (These end up on *Machines*.)
		if len(ms.Spec.Template.ObjectMeta.Labels) == 0 {
			// This should never actually happen, as the generators (for all platforms)
			// add labels and corresponding selectors used by MAPI.
			ms.Spec.Template.ObjectMeta.Labels = map[string]string{}
		}
		for k, v := range pool.Spec.MachineLabels {
			// Don't allow the user to shoot themselves in the foot by overriding generated labels.
			if _, exists := ms.Spec.Template.ObjectMeta.Labels[k]; exists {
				logger.WithField("machineLabel", k).Warnf("conflict with generated label -- ignoring")
				generatedMachineLabels.Insert(k)
				continue
			}
			ms.Spec.Template.ObjectMeta.Labels[k] = v
		}
	}

	logger.WithField("numMachineSets", len(generatedMachineSets)).Info("generated worker machine sets")

	return generatedMachineSets, generatedMachineLabels, true, nil
}

// ensureEnoughReplicas ensures that the min replicas in the machine pool is
// large enough to cover all of the zones for the machine pool. When using
// auto-scaling for some platforms, every machineset needs to have a minimum replicas of 1.
// If the reconcile.Result returned in non-nil, then the reconciliation loop
// should stop, returning that result. This is used to prevent re-queueing
// a machine pool that will always fail with not enough replicas. There is
// nothing that the controller can do with such a machine pool until the user
// makes updates to the machine pool.
func (r *ReconcileMachinePool) ensureEnoughReplicas(
	pool *hivev1.MachinePool,
	generatedMachineSets []*machineapi.MachineSet,
	cd *hivev1.ClusterDeployment,
	logger log.FieldLogger,
) (*reconcile.Result, error) {
	if pool.Spec.Autoscaling != nil {
		// Handle autoscaling case: check if we have enough replicas
		if pool.Spec.Autoscaling.MinReplicas < int32(len(generatedMachineSets)) && !platformAllowsZeroAutoscalingMinReplicas(cd) {
			logger.WithField("machinesets", len(generatedMachineSets)).
				WithField("minReplicas", pool.Spec.Autoscaling.MinReplicas).
				Warning("when auto-scaling, the MachinePool must have at least one replica for each MachineSet")
			err := r.updateCondition(
				pool,
				hivev1.NotEnoughReplicasMachinePoolCondition,
				corev1.ConditionTrue,
				"MinReplicasTooSmall",
				fmt.Sprintf("When auto-scaling, the MachinePool must have at least one replica for each MachineSet. The minReplicas must be at least %d", len(generatedMachineSets)),
				logger,
			)
			return &reconcile.Result{}, err
		}
		// Autoscaling enabled and we have enough replicas
		err := r.updateCondition(
			pool,
			hivev1.NotEnoughReplicasMachinePoolCondition,
			corev1.ConditionFalse,
			"EnoughReplicas",
			"The MachinePool has sufficient replicas for each MachineSet",
			logger,
		)
		return nil, err
	}

	// When autoscaling is disabled, reset the NotEnoughReplicas condition to initialized state
	err := r.updateCondition(
		pool,
		hivev1.NotEnoughReplicasMachinePoolCondition,
		corev1.ConditionUnknown,
		hivev1.InitializedConditionReason,
		"Condition Initialized",
		logger,
	)
	return nil, err
}

// Compare Failure Domains to confirm that they match
func matchFailureDomains(gMS *machineapi.MachineSet, rMS machineapi.MachineSet, infrastructure *configv1.Infrastructure, logger log.FieldLogger) (bool, error) {
	rSpec := rMS.Spec.Template.Spec
	gSpec := gMS.Spec.Template.Spec
	var err error

	if rSpec.ProviderSpec.Value == nil {
		return false, errors.New("remote MachineSet provider spec is nil")
	}
	if gSpec.ProviderSpec.Value == nil {
		return false, errors.New("generated MachineSet provider spec is nil")
	}
	// cpms utils operate on the Raw value of the provider spec rather than the Object,
	// and internally unmarshal the raw value into the Object.
	// Ensure that Raw is populated before calling cpms utils.
	// TODO remove this once cpms utils operate on the Object instead of the Raw value.
	if rSpec.ProviderSpec.Value.Raw == nil {
		rSpec.ProviderSpec.Value.Raw, err = json.Marshal(rMS.Spec.Template.Spec.ProviderSpec.Value.Object)
		if err != nil {
			logger.WithError(err).Error("unable to marshal remote ms provider spec object to raw value")
			return false, err
		}
	}
	if gSpec.ProviderSpec.Value.Raw == nil {
		gSpec.ProviderSpec.Value.Raw, err = json.Marshal(gMS.Spec.Template.Spec.ProviderSpec.Value.Object)
		if err != nil {
			logger.WithError(err).Error("unable to marshal generate ms provider spec object to raw value")
			return false, err
		}
	}

	// - The provider config funcs take a different kind of logger. Convert.
	logr := logrus.NewLogr(logger)
	rMS_providerconfig, err := cpms.NewProviderConfigFromMachineSpec(logr, rSpec, infrastructure)
	if err != nil {
		logger.WithError(err).Errorf("unable to parse remote MachineSet %v provider config", rMS.Name)
		return false, err
	}
	gMS_providerconfig, err := cpms.NewProviderConfigFromMachineSpec(logr, gSpec, infrastructure)
	if err != nil {
		logger.WithError(err).Errorf("unable to parse generated MachineSet %v provider config", gMS.Name)
		return false, err
	}
	rfd := rMS_providerconfig.ExtractFailureDomain()
	gfd := gMS_providerconfig.ExtractFailureDomain()

	rfdtype, gfdtype := rfd.Type(), gfd.Type()
	// This should never happen
	if rfdtype != gfdtype {
		return false, errors.Errorf("Unexpectedly got differing failure domain types for remote (%s) and generated (%s) MachineSets!", rfdtype, gfdtype)
	}
	// HIVE-2443: Special case for AWS: Failure domains for AWS contain a subnet which may be
	// described by ID, ARN, or Filters. These can't be reliably compared. However, MachineSets are
	// generated/named based only on the AZ name, so we'll just compare that and ignore the subnet.
	if rfdtype == configv1.AWSPlatformType { // only necessary to check one since we verified they're equal above
		return rfd.AWS().Placement.AvailabilityZone == gfd.AWS().Placement.AvailabilityZone, nil
	}

	// Otherwise the FailureDomain should be unambiguous and we can just compare them.
	equal := rfd.Equal(gfd)
	if !equal {
		logger.WithField("failureDomainRemote", rfd).WithField("failureDomainGenerated", gfd).Info("failure domains not equal")
	}
	return equal, nil
}

// matchMachineSets decides whether gMS ("generated MachineSet") and rMS ("remote MachineSet" -- the one already extant
// on the spoke cluster) are talking about the same machines. In certain cases, this may be true when
// the names don't match. The function therefore relies on the hive machinePoolNameLabel to determine whether the MachineSets
// are part of the same MachinePool. If the machinePoolNameLabels match, the function then confirms that the MachineSets belong
// to the same Availability Zone (aka Failure Domain). This ensures that the MachineSets are referring to the same machines.
// We can count on this because the upsteam generator guarentees at most one MachineSet per Failure Domain. HIVE-2254.
func matchMachineSets(gMS *machineapi.MachineSet, rMS machineapi.MachineSet, infrastructure *configv1.Infrastructure, logger log.FieldLogger) (bool, error) {

	gLabel, gLabelExists := gMS.Labels[machinePoolNameLabel]
	rLabel, rLabelExists := rMS.Labels[machinePoolNameLabel]

	if !gLabelExists {
		// panic because this should never happen
		panic(fmt.Sprintf("generated MachineSet %v does not have a machinePoolNameLabel", gMS.Name))
	}
	if !rLabelExists {
		return false, nil
	}
	if gLabel != rLabel {
		return false, nil
	}

	return matchFailureDomains(gMS, rMS, infrastructure, logger)
}

func (r *ReconcileMachinePool) syncMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	generatedMachineSets []*machineapi.MachineSet,
	remoteMachineSets *machineapi.MachineSetList,
	infrastructure *configv1.Infrastructure,
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) ([]*machineapi.MachineSet, error) {
	result := make([]*machineapi.MachineSet, len(generatedMachineSets))

	machineSetsToDelete := []*machineapi.MachineSet{}
	machineSetsToCreate := []*machineapi.MachineSet{}
	machineSetsToUpdate := []*machineapi.MachineSet{}

	// Compile a set of labels and taints that were owned by MachinePool but have now been removed from MachinePool.Spec.
	// These would need to be deleted from the remoteMachineSet if present.
	labelsToDelete := sets.Set[string]{}
	labelsToDelete.Insert(pool.Status.OwnedLabels...)
	for labelKey := range pool.Spec.Labels {
		labelsToDelete.Delete(labelKey)
	}
	machineLabelsToDelete := sets.Set[string]{}
	machineLabelsToDelete.Insert(pool.Status.OwnedMachineLabels...)
	for labelKey := range pool.Spec.MachineLabels {
		machineLabelsToDelete.Delete(labelKey)
	}
	taintsToDelete := sets.Set[hivev1.TaintIdentifier]{}
	taintsToDelete.Insert(pool.Status.OwnedTaints...)
	for _, taint := range pool.Spec.Taints {
		taintsToDelete.Delete(controllerutils.IdentifierForTaint(&taint))
	}

	// Find MachineSets that need updating/creating
	for i, ms := range generatedMachineSets {
		found := false
		for _, rMS := range remoteMachineSets.Items {
			var err error
			found, err = matchMachineSets(ms, rMS, infrastructure, logger)
			// Labels matched but there was still an error
			// In order to prevent the creation of a duplicate MachineSet, error out and do not continue matching.
			if err != nil {
				logger.WithError(err).Errorf("unable to match MachineSets %s and %s", ms.Name, rMS.Name)
				return nil, err
			}
			if found {
				logger.Infof("Found matching remote MachineSet %s for generated MachineSet %s", rMS.Name, ms.Name)
				objectModified := false
				objectMetaModified := false
				// Rename generated MachineSet if there is a name mismatch
				// The rename doesn't currently serve a functional purpose, but may assist in future cases where it gets used.
				// TODO: Determine how to handle a case where a remote machineSet has the same name as a different (non-matching) generated machineSet.
				// In particular, this would result in a discrepancy in the machine.openshift.io/cluster-api-machineset label!
				ms.Name = rMS.Name
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

				mergeLabels := func(rLabels, gLabels map[string]string, labelsToDelete sets.Set[string]) (map[string]string, bool) {
					mod := false
					// Carry over the labels on the remote machineset from the generated machineset. Merging strategy preserves the unique labels of remote machineset.
					if rLabels == nil {
						rLabels = make(map[string]string)
					}
					// First delete the labels that need to be deleted, then sync the remoteMachineSet labels with that of the generatedMachineSet.
					for key := range rLabels {
						if labelsToDelete.Has(key) {
							// Safe to delete while iterating as long as we're deleting the key for this iteration.
							delete(rLabels, key)
							mod = true
						}
					}
					for key, value := range gLabels {
						if val, ok := rLabels[key]; !ok || val != value {
							// Special case! If we matched despite a name conflict, do not update the cluster-api-machineset
							// label, as it must match the *remote* machineset name.
							if key == capiMachineSetKey {
								continue
							}
							rLabels[key] = value
							mod = true
						}
					}
					return rLabels, mod
				}
				var mod bool
				if rMS.Spec.Template.Spec.Labels, mod = mergeLabels(rMS.Spec.Template.Spec.Labels, ms.Spec.Template.Spec.Labels, labelsToDelete); mod {
					objectModified = true
				}
				if rMS.Spec.Template.Labels, mod = mergeLabels(rMS.Spec.Template.Labels, ms.Spec.Template.Labels, machineLabelsToDelete); mod {
					objectModified = true
				}

				// Carry over the taints on the remote machineset from the generated machineset. Merging strategy preserves the unique taints of remote machineset.
				// First, build a map of remote MachineSet taints, so we simultaneously collapse duplicate entries.
				taintMap := make(map[hivev1.TaintIdentifier]corev1.Taint)
				for _, rTaint := range rMS.Spec.Template.Spec.Taints {
					if taintsToDelete.Has(controllerutils.IdentifierForTaint(&rTaint)) {
						// This entry would be deleted from the final list
						objectModified = true
						continue
					}
					// Preserve the taint entry first encountered.
					if !controllerutils.TaintExists(taintMap, &rTaint) {
						taintMap[controllerutils.IdentifierForTaint(&rTaint)] = rTaint
					} else {
						// Skip this taint, which effectively means we're removing a duplicate.
						objectModified = true
					}
				}
				for _, gTaint := range ms.Spec.Template.Spec.Taints {
					// Generated MachineSet will not have duplicate taints, because we have collapsed duplicates when we generated them.
					if !controllerutils.TaintExists(taintMap, &gTaint) || taintMap[controllerutils.IdentifierForTaint(&gTaint)] != gTaint {
						taintMap[controllerutils.IdentifierForTaint(&gTaint)] = gTaint
						objectModified = true
					}
				}
				rMS.Spec.Template.Spec.Taints = *controllerutils.ListFromTaintMap(&taintMap)

				// Platform updates will be blocked by webhook, unless they're not.
				if !reflect.DeepEqual(rMS.Spec.Template.Spec.ProviderSpec.Value, ms.Spec.Template.Spec.ProviderSpec.Value) {
					msg := "ProviderSpec out of sync"
					if mutable, err := strconv.ParseBool(pool.Annotations[constants.OverrideMachinePoolPlatformAnnotation]); err != nil || !mutable {
						msLog.Warning(msg)
					} else {
						msLog.Info(msg)
						rMS.Spec.Template.Spec.ProviderSpec.Value = ms.Spec.Template.Spec.ProviderSpec.Value
						objectModified = true
					}
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

func (r *ReconcileMachinePool) syncMachineAutoscalers(
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
		&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}},
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
			for j, rMA := range remoteMachineAutoscalers.Items {
				if ms.Name == rMA.Name {
					found = true
					objectModified := false
					maLog := logger.WithField("machineautoscaler", rMA.Name)

					if rMA.Spec.MinReplicas != minReplicas {
						maLog.WithField("desired", minReplicas).
							WithField("observed", rMA.Spec.MinReplicas).
							Info("min replicas out of sync")
						remoteMachineAutoscalers.Items[j].Spec.MinReplicas = minReplicas
						objectModified = true
					}

					if rMA.Spec.MaxReplicas != maxReplicas {
						maLog.WithField("desired", maxReplicas).
							WithField("observed", rMA.Spec.MaxReplicas).
							Info("max replicas out of sync")
						remoteMachineAutoscalers.Items[j].Spec.MaxReplicas = maxReplicas
						objectModified = true
					}

					// A MachineAutoscaler can't have MaxReplicas==0. In that case, update will
					// bounce. We must delete the MachineAutoscaler instead, which we trigger
					// below based on having set MaxReplicas to zero above.
					if objectModified && maxReplicas > 0 {
						machineAutoscalersToUpdate = append(machineAutoscalersToUpdate, &remoteMachineAutoscalers.Items[j])
					}
					break
				}
			}

			// Don't attempt to create a MachineAutoscaler with MaxReplicas==0.
			if !found && maxReplicas > 0 {
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
					// A MachineAutoscaler can't have MaxReplicas==0. Delete it if that's the case.
					if rMA.Spec.MaxReplicas > 0 {
						delete = false
					}
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

func (r *ReconcileMachinePool) syncClusterAutoscaler(
	pool *hivev1.MachinePool,
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
		&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}},
	); err != nil {
		logger.WithError(err).Error("unable to fetch remote cluster autoscalers")
		return err
	}
	logger.Infof("found %v remote cluster autoscalers", len(remoteClusterAutoscalers.Items))
	var defaultClusterAutoscaler *autoscalingv1.ClusterAutoscaler
	for _, rCA := range remoteClusterAutoscalers.Items {
		if rCA.Name == "default" {
			// If an existing ClusterAutoscaler object found - leave it untouched
			return nil
		}
	}

	logger.Info("creating cluster autoscaler")
	defaultClusterAutoscaler = &autoscalingv1.ClusterAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
		Spec: autoscalingv1.ClusterAutoscalerSpec{
			ScaleDown: &autoscalingv1.ScaleDownConfig{
				Enabled: true,
			},
			BalanceSimilarNodeGroups: pointer.Bool(true),
		},
	}
	if err := remoteClusterAPIClient.Create(context.Background(), defaultClusterAutoscaler); err != nil {
		logger.WithError(err).Error("could not create cluster autoscaler")
		return err
	}

	return nil
}

func (r *ReconcileMachinePool) updatePoolStatusForMachineSets(
	pool *hivev1.MachinePool,
	machineSets []*machineapi.MachineSet,
	generatedMachineLabels sets.Set[string],
	remoteClusterAPIClient client.Client,
	logger log.FieldLogger,
) (reconcile.Result, error) {
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
		s := hivev1.MachineSetStatus{
			Name:          ms.Name,
			Replicas:      *ms.Spec.Replicas,
			ReadyReplicas: ms.Status.ReadyReplicas,
			MinReplicas:   min,
			MaxReplicas:   max,
			ErrorReason:   (*string)(ms.Status.ErrorReason),
			ErrorMessage:  ms.Status.ErrorMessage,
		}
		if s.Replicas != s.ReadyReplicas && s.ErrorReason == nil {
			r, m := summarizeMachinesError(remoteClusterAPIClient, ms, logger)
			s.ErrorReason = &r
			s.ErrorMessage = &m
		}

		pool.Status.MachineSets[i] = s
		pool.Status.Replicas += *ms.Spec.Replicas
	}

	var requeueAfter time.Duration
	for _, ms := range pool.Status.MachineSets {
		if ms.Replicas != ms.ReadyReplicas {
			// remote cluster machinesets cannot trigger reconcile and therefore
			// since we know we are not steady state, we need to ensure that we
			// requeue to keep the status in sync from remote cluster.
			requeueAfter = 10 * time.Minute
			break
		}
	}

	pool.Status = updateOwnedLabelsAndTaints(pool, generatedMachineLabels)

	if (len(origPool.Status.MachineSets) == 0 && len(pool.Status.MachineSets) == 0) ||
		reflect.DeepEqual(origPool.Status, pool.Status) {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, errors.Wrap(r.Status().Update(context.Background(), pool), "failed to update pool status")
}

// updateOwnedLabelsAndTaints updates OwnedLabels and OwnedTaints in the MachinePool.Status, by fetching the relevant entries sans duplicates from MachinePool.Spec.
func updateOwnedLabelsAndTaints(pool *hivev1.MachinePool, generatedMachineLabels sets.Set[string]) hivev1.MachinePoolStatus {
	// Update our tracked labels...
	getOwnedLabels := func(labels map[string]string, excludes sets.Set[string]) []string {
		ownedLabels := sets.Set[string]{}
		for labelKey := range labels {
			if excludes.Has(labelKey) {
				continue
			}
			ownedLabels.Insert(labelKey)
		}
		// sets.List sorts
		return sets.List(ownedLabels)
	}
	pool.Status.OwnedLabels = getOwnedLabels(pool.Spec.Labels, nil)
	// NOTE: This only claims "ownership" of user-specified labels. Generated labels
	// will be asserted regardless.
	pool.Status.OwnedMachineLabels = getOwnedLabels(pool.Spec.MachineLabels, generatedMachineLabels)

	// ...and taints
	uniqueTaints := *controllerutils.GetUniqueTaints(&pool.Spec.Taints)
	ownedTaints := make([]hivev1.TaintIdentifier, len(uniqueTaints))
	for i, taint := range uniqueTaints {
		ownedTaints[i] = controllerutils.IdentifierForTaint(&taint)
	}
	sort.Slice(ownedTaints, func(i, j int) bool {
		// It is not important that these be actually "sorted" -- just that they are
		// ordered deterministically.
		return fmt.Sprintf("%v", ownedTaints[i]) < fmt.Sprintf("%v", ownedTaints[j])
	})
	pool.Status.OwnedTaints = ownedTaints
	return pool.Status
}

// summarizeMachinesError returns reason and message for error state of machineSets by
// summarizing error reasons and messages from machines the belong to the machineset.
// If all the machines are in good state, it returns empty reason and message.
func summarizeMachinesError(remoteClusterAPIClient client.Client, machineSet *machineapi.MachineSet, logger log.FieldLogger) (string, string) {
	msLog := logger.WithField("machineSet", machineSet.Name)

	sel, err := metav1.LabelSelectorAsSelector(&machineSet.Spec.Selector)
	if err != nil {
		msLog.WithError(err).Error("failed to create label selector")
		return "", ""
	}

	list := &machineapi.MachineList{}
	err = remoteClusterAPIClient.List(context.TODO(), list,
		client.InNamespace(machineSet.GetNamespace()),
		client.MatchingLabelsSelector{Selector: sel})
	if err != nil {
		msLog.WithError(err).Error("failed to list machines for the machineset")
		return "", ""
	}

	type errSummary struct {
		name    string
		reason  string
		message string
	}

	var errs []errSummary
	for _, m := range list.Items {
		if m.Status.ErrorReason == nil &&
			m.Status.ErrorMessage == nil {
			continue
		}

		err := errSummary{name: m.GetName()}
		if m.Status.ErrorReason != nil {
			err.reason = string(*m.Status.ErrorReason)
		}
		if m.Status.ErrorMessage != nil {
			err.message = *m.Status.ErrorMessage
		}
		errs = append(errs, err)
	}

	if len(errs) == 1 {
		return errs[0].reason, errs[0].message
	}
	if len(errs) > 1 {
		r := "MultipleMachinesFailed"
		m := &bytes.Buffer{}
		for _, e := range errs {
			fmt.Fprintf(m, "Machine %s failed (%s): %s,\n", e.name, e.reason, e.message)
		}
		return r, m.String()
	}
	return "", ""
}

func (r *ReconcileMachinePool) createActuator(
	cd *hivev1.ClusterDeployment,
	pool *hivev1.MachinePool,
	masterMachine *machineapi.Machine,
	remoteMachineSets []machineapi.MachineSet,
	logger log.FieldLogger,
) (Actuator, error) {
	switch {
	case cd.Spec.Platform.AWS != nil:
		creds := awsclient.CredentialsSource{
			Secret: &awsclient.SecretCredentialsSource{
				Ref:       &cd.Spec.Platform.AWS.CredentialsSecretRef,
				Namespace: cd.Namespace,
			},
			AssumeRole: &awsclient.AssumeRoleCredentialsSource{
				SecretRef: corev1.SecretReference{
					Namespace: controllerutils.GetHiveNamespace(),
					Name:      controllerutils.AWSServiceProviderSecretName(""),
				},
				Role: cd.Spec.Platform.AWS.CredentialsAssumeRole,
			},
		}
		return NewAWSActuator(r.Client, creds, cd.Spec.Platform.AWS.Region, pool, masterMachine, r.scheme, logger)
	case cd.Spec.Platform.Azure != nil:
		creds := &corev1.Secret{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      cd.Spec.Platform.Azure.CredentialsSecretRef.Name,
				Namespace: cd.Namespace,
			},
			creds,
		); err != nil {
			return nil, err
		}
		return NewAzureActuator(creds, cd.Spec.Platform.Azure.CloudName.Name(), logger)
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
		return NewGCPActuator(r.Client, creds, pool, masterMachine, remoteMachineSets, r.scheme, r.expectations, logger)
	case cd.Spec.Platform.IBMCloud != nil:
		creds := &corev1.Secret{}
		if err := r.Get(
			context.TODO(),
			types.NamespacedName{
				Name:      cd.Spec.Platform.IBMCloud.CredentialsSecretRef.Name,
				Namespace: cd.Namespace,
			},
			creds,
		); err != nil {
			return nil, err
		}
		return NewIBMCloudActuator(creds, r.scheme, logger)
	case cd.Spec.Platform.Nutanix != nil:
		return NewNutanixActuator(r.Client, masterMachine)
	case cd.Spec.Platform.Ovirt != nil:
		return NewOvirtActuator(masterMachine, r.scheme, logger)
	case cd.Spec.Platform.OpenStack != nil:
		return NewOpenStackActuator(masterMachine, r.Client, logger)
	case cd.Spec.Platform.VSphere != nil:
		return NewVSphereActuator(masterMachine, r.scheme, logger)
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

func (r *ReconcileMachinePool) removeFinalizer(pool *hivev1.MachinePool, logger log.FieldLogger) (reconcile.Result, error) {
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

func getClusterVersion(cd *hivev1.ClusterDeployment) (string, error) {
	version, versionPresent := cd.Labels[constants.VersionLabel]
	if !versionPresent {
		return "", errors.New("cluster version not set in clusterdeployment")
	}
	return version, nil
}

func platformAllowsZeroAutoscalingMinReplicas(cd *hivev1.ClusterDeployment) bool {
	// Zero-sized minReplicas for autoscaling are allowed since OCP:
	// - 4.5 for AWS, Azure, and GCP
	// - 4.7 for OpenStack
	if cd.Spec.Platform.AWS != nil || cd.Spec.Platform.Azure != nil || cd.Spec.Platform.GCP != nil || cd.Spec.Platform.OpenStack != nil {
		return true
	}

	return false
}

// IsErrorUpdateEvent returns true when the update event for MachinePool is from
// error state.
func IsErrorUpdateEvent(evt event.UpdateEvent) bool {
	new, ok := evt.ObjectNew.(*hivev1.MachinePool)
	if !ok {
		return false
	}
	if len(new.Status.Conditions) == 0 {
		return false
	}

	old, ok := evt.ObjectOld.(*hivev1.MachinePool)
	if !ok {
		return false
	}

	errorConds := []hivev1.MachinePoolConditionType{
		hivev1.InvalidSubnetsMachinePoolCondition,
		hivev1.UnsupportedConfigurationMachinePoolCondition,
	}

	for _, cond := range errorConds {
		cn := controllerutils.FindCondition(new.Status.Conditions, cond)
		if cn != nil && cn.Status == corev1.ConditionTrue {
			co := controllerutils.FindCondition(old.Status.Conditions, cond)
			if co == nil {
				return true // newly added failure condition
			}
			if co.Status != corev1.ConditionTrue {
				return true // newly Failed failure condition
			}
			if cn.Message != co.Message ||
				cn.Reason != co.Reason {
				return true // already failing but change in error reported
			}
		}
	}

	return false
}

func (r *ReconcileMachinePool) updateCondition(
	pool *hivev1.MachinePool,
	cond hivev1.MachinePoolConditionType,
	status corev1.ConditionStatus,
	reason, message string,
	logger log.FieldLogger,
) error {
	conds, changed := controllerutils.SetMachinePoolConditionWithChangeCheck(
		pool.Status.Conditions,
		cond,
		status,
		reason,
		message,
		controllerutils.UpdateConditionIfReasonOrMessageChange,
	)
	if changed {
		pool.Status.Conditions = conds
		if err := r.Status().Update(context.Background(), pool); err != nil {
			logger.WithError(err).Error("failed to update MachinePool conditions")
			return err
		}
	}
	return nil
}

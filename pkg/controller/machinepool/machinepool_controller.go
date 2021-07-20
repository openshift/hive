package machinepool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	capiv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	autoscalingv1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1"
	autoscalingv1beta1 "github.com/openshift/cluster-autoscaler-operator/pkg/apis/autoscaling/v1beta1"
	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	machineapi "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/remoteclient"
)

const (
	ControllerName             = hivev1.RemoteMachinesetControllerName
	machinePoolNameLabel       = "hive.openshift.io/machine-pool"
	finalizer                  = "hive.openshift.io/remotemachineset"
	masterMachineLabelSelector = "machine.openshift.io/cluster-api-machine-type=master"
)

var (
	// controllerKind contains the schema.GroupVersionKind for this controller type.
	controllerKind = hivev1.SchemeGroupVersion.WithKind("MachinePool")

	// remoteMachineSetConditions are the Machine Pool conditions controlled by remote machineset controller
	remoteMachineSetConditions = []hivev1.MachinePoolConditionType{
		hivev1.NotEnoughReplicasMachinePoolCondition,
		hivev1.NoMachinePoolNameLeasesAvailable,
		hivev1.InvalidSubnetsMachinePoolCondition,
		hivev1.UnsupportedConfigurationMachinePoolCondition,
	}
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)

	scheme := mgr.GetScheme()
	if err := capiv1.AddToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add capiv1 to scheme")
	}
	if err := addAWSProviderToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add AWS provider to scheme")
	}
	if err := addGCPProviderToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add GCP provider to scheme")
	}
	if err := addOpenStackProviderToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add OpenStack provider to scheme")
	}
	if err := addOvirtProviderToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add OVirt provider to scheme")
	}
	if err := addVSphereProviderToScheme(scheme); err != nil {
		return errors.Wrap(err, "cannot add vSphere provider to scheme")
	}
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}

	r := &ReconcileMachinePool{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &clientRateLimiter),
		scheme:       mgr.GetScheme(),
		logger:       logger,
		expectations: controllerutils.NewExpectations(logger),
	}
	r.actuatorBuilder = func(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, masterMachine *machineapi.Machine, remoteMachineSets []machineapi.MachineSet, logger log.FieldLogger) (Actuator, error) {
		return r.createActuator(cd, pool, masterMachine, remoteMachineSets, logger)
	}
	r.remoteClusterAPIClientBuilder = func(cd *hivev1.ClusterDeployment) remoteclient.Builder {
		return remoteclient.NewBuilder(r.Client, cd, ControllerName)
	}

	// Create a new controller
	c, err := controller.New("remotemachineset-controller", mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             queueRateLimiter,
	})
	if err != nil {
		return err
	}

	// Watch for changes to MachinePools
	err = c.Watch(&source.Kind{Type: &hivev1.MachinePool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for MachinePoolNameLeases created for some MachinePools (currently just GCP):
	if err := r.watchMachinePoolNameLeases(c); err != nil {
		return errors.Wrap(err, "could not watch MachinePoolNameLeases")
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, handler.EnqueueRequestsFromMapFunc(
		r.clusterDeploymentWatchHandler,
	))
	if err != nil {
		return err
	}

	// Periodically watch MachinePools for syncing status from external clusters
	err = c.Watch(newPeriodicSource(r.Client, 30*time.Minute, r.logger), &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileMachinePool) clusterDeploymentWatchHandler(a client.Object) []reconcile.Request {
	retval := []reconcile.Request{}

	cd := a.(*hivev1.ClusterDeployment)
	if cd == nil {
		// Wasn't a clusterdeployment, bail out. This should not happen.
		r.logger.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a)
		return retval
	}

	pools := &hivev1.MachinePoolList{}
	err := r.List(context.TODO(), pools)
	if err != nil {
		// Could not list machine pools
		r.logger.Errorf("Error listing machine pools. Value: %+v", a)
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

// ReconcileMachinePool reconciles the MachineSets generated from a ClusterDeployment object
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

	// Initialize machine pool conditions if not present
	newConditions := controllerutils.InitializeMachinePoolConditions(pool.Status.Conditions, remoteMachineSetConditions)
	if len(newConditions) > len(pool.Status.Conditions) {
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

	if controllerutils.IsFakeCluster(cd) {
		logger.Info("skipping reconcile for fake cluster")
		return reconcile.Result{}, nil
	}

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

	masterMachine, err := r.getMasterMachine(cd, remoteClusterAPIClient, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	if cd.Spec.Platform.AWS != nil && cd.Spec.MachineManagement != nil && cd.Spec.MachineManagement.Central != nil {
		logger.Info("reconciling local machinesets")
		return r.reconcileLocalMachineSets(pool, cd, masterMachine, logger)
	}

	logger.Info("reconciling remote machinesets")
	return r.reconcileRemoteMachineSets(pool, cd, masterMachine, remoteClusterAPIClient, logger)
}

func (r *ReconcileMachinePool) reconcileLocalMachineSets(pool *hivev1.MachinePool, cd *hivev1.ClusterDeployment, masterMachine *machineapi.Machine, logger log.FieldLogger) (reconcile.Result, error) {
	localMachineSets, err := r.getLocalMachineSets(logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not getLocalMachineSets")
		return reconcile.Result{}, err
	}

	generatedMachineSets, proceed, err := r.generateCAPIMachineSets(pool, cd, masterMachine, localMachineSets, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not generateCAPIMachineSets")
		return reconcile.Result{}, err
	} else if !proceed {
		logger.Info("machineSets generator indicated not to proceed, returning")
		return reconcile.Result{}, nil
	}

	switch result, err := r.ensureEnoughReplicasCAPI(pool, generatedMachineSets, cd, logger); {
	case err != nil:
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not ensureEnoughReplicasCAPI")
		return reconcile.Result{}, err
	case result != nil:
		return *result, nil
	}

	logger.Info("syncing machinesets")
	_, err = r.syncCAPIMachineSets(pool, cd, generatedMachineSets, localMachineSets, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncMachineSets")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileMachinePool) reconcileRemoteMachineSets(pool *hivev1.MachinePool, cd *hivev1.ClusterDeployment, masterMachine *machineapi.Machine, remoteClusterAPIClient client.Client, logger log.FieldLogger) (reconcile.Result, error) {
	remoteMachineSets, err := r.getRemoteMachineSets(remoteClusterAPIClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not getRemoteMachineSets")
		return reconcile.Result{}, err
	}

	generatedMachineSets, proceed, err := r.generateMachineSets(pool, cd, masterMachine, remoteMachineSets, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not generateMachineSets")
		return reconcile.Result{}, err
	} else if !proceed {
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

	machineSets, err := r.syncMachineSets(pool, cd, generatedMachineSets, remoteMachineSets, remoteClusterAPIClient, logger)
	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncMachineSets")
		return reconcile.Result{}, err
	}

	if err := r.syncMachineAutoscalers(pool, cd, machineSets, remoteClusterAPIClient, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncMachineAutoscalers")
		return reconcile.Result{}, err
	}

	if err := r.syncClusterAutoscaler(pool, cd, remoteClusterAPIClient, logger); err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "could not syncClusterAutoscaler")
		return reconcile.Result{}, err
	}

	if pool.DeletionTimestamp != nil {
		return r.removeFinalizer(pool, logger)
	}

	return r.updatePoolStatusForMachineSets(pool, machineSets, remoteClusterAPIClient, logger)
}

func (r *ReconcileMachinePool) getMasterMachine(
	cd *hivev1.ClusterDeployment,
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

func (r *ReconcileMachinePool) getLocalMachineSets(
	logger log.FieldLogger,
) (*capiv1.MachineSetList, error) {
	localMachineSets := &capiv1.MachineSetList{}
	if err := capiv1.AddToScheme(r.scheme); err != nil {
		return nil, err
	}
	if err := r.Client.List(
		context.Background(),
		localMachineSets,
	); err != nil {
		logger.WithError(err).Error("unable to fetch local machine sets")
		return nil, err
	}
	logger.Infof("found %v local machine sets", len(localMachineSets.Items))
	return localMachineSets, nil
}

func (r *ReconcileMachinePool) generateMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	masterMachine *machineapi.Machine,
	remoteMachineSets *machineapi.MachineSetList,
	logger log.FieldLogger,
) ([]*machineapi.MachineSet, bool, error) {
	if pool.DeletionTimestamp != nil {
		return nil, true, nil
	}

	actuator, err := r.actuatorBuilder(cd, pool, masterMachine, remoteMachineSets.Items, logger)
	if err != nil {
		logger.WithError(err).Error("unable to create actuator")
		return nil, false, err
	}

	generatedMachineSets, proceed, err := actuator.GenerateMachineSets(cd, pool, logger)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not generate machinesets")
	} else if !proceed {
		logger.Info("actuator indicated not to proceed, returning")
		return nil, false, nil
	}

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
		ms.Spec.Template.Spec.ObjectMeta.Labels = make(map[string]string, len(pool.Spec.Labels))
		for key, value := range pool.Spec.Labels {
			ms.Spec.Template.Spec.ObjectMeta.Labels[key] = value
		}

		// Apply hive MachinePool taints to MachineSet MachineSpec.
		ms.Spec.Template.Spec.Taints = pool.Spec.Taints
	}

	logger.Infof("generated %v worker machine sets", len(generatedMachineSets))

	return generatedMachineSets, true, nil
}

func (r *ReconcileMachinePool) generateCAPIMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	masterMachine *machineapi.Machine,
	localMachineSets *capiv1.MachineSetList,
	logger log.FieldLogger,
) ([]*capiv1.MachineSet, bool, error) {
	if pool.DeletionTimestamp != nil {
		return nil, true, nil
	}

	dummyMachineSets := machineapi.MachineSetList{}
	actuator, err := r.actuatorBuilder(cd, pool, masterMachine, dummyMachineSets.Items, logger)
	if err != nil {
		logger.WithError(err).Error("unable to create actuator")
		return nil, false, err
	}

	logger.Info("generating CAPI machinesets")
	generatedCAPIMachineSets, proceed, err := actuator.GenerateCAPIMachineSets(cd, pool, logger)
	if err != nil {
		return nil, false, errors.Wrap(err, "could not generate machinesets")
	} else if !proceed {
		logger.Info("actuator indicated not to proceed, returning")
	}
	if generatedCAPIMachineSets == nil {
		return nil, false, fmt.Errorf("didnt get any CAPI machinesets :)")
	}

	return generatedCAPIMachineSets, true, nil
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
	if pool.Spec.Autoscaling == nil {
		return nil, nil
	}
	if pool.Spec.Autoscaling.MinReplicas < int32(len(generatedMachineSets)) && !platformAllowsZeroAutoscalingMinReplicas(cd) {
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
		"The MachinePool has sufficient replicas for each MachineSet",
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

func (r *ReconcileMachinePool) ensureEnoughReplicasCAPI(
	pool *hivev1.MachinePool,
	generatedMachineSets []*capiv1.MachineSet,
	cd *hivev1.ClusterDeployment,
	logger log.FieldLogger,
) (*reconcile.Result, error) {
	if pool.Spec.Autoscaling == nil {
		return nil, nil
	}
	if pool.Spec.Autoscaling.MinReplicas < int32(len(generatedMachineSets)) && !platformAllowsZeroAutoscalingMinReplicas(cd) {
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
		"The MachinePool has sufficient replicas for each MachineSet",
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

func (r *ReconcileMachinePool) syncMachineSets(
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

func (r *ReconcileMachinePool) syncCAPIMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	generatedMachineSets []*capiv1.MachineSet,
	localMachineSets *capiv1.MachineSetList,
	logger log.FieldLogger,
) ([]*capiv1.MachineSet, error) {
	result := make([]*capiv1.MachineSet, len(generatedMachineSets))

	machineSetsToDelete := []*capiv1.MachineSet{}
	machineSetsToCreate := []*capiv1.MachineSet{}
	machineSetsToUpdate := []*capiv1.MachineSet{}

	// Find MachineSets that need updating/creating
	for i, ms := range generatedMachineSets {
		found := false
		for _, rMS := range localMachineSets.Items {
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
					min, max := getMinMaxReplicasForCAPIMachineSet(pool, generatedMachineSets, i)
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

				/*
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
				*/

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
	for i, rMS := range localMachineSets.Items {
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
			machineSetsToDelete = append(machineSetsToDelete, &localMachineSets.Items[i])
		}
	}

	for _, ms := range machineSetsToCreate {
		logger.WithField("machineset", ms.Name).Info("creating machineset")
		if err := r.Client.Create(context.Background(), ms); err != nil {
			logger.WithError(err).Error("unable to create machine set")
			return nil, err
		}
	}

	for _, ms := range machineSetsToUpdate {
		logger.WithField("machineset", ms.Name).Info("updating machineset")
		if err := r.Client.Update(context.Background(), ms); err != nil {
			logger.WithError(err).Error("unable to update machine set")
			return nil, err
		}
	}

	for _, ms := range machineSetsToDelete {
		logger.WithField("machineset", ms.Name).Info("deleting machineset")
		if err := r.Client.Delete(context.Background(), ms); err != nil {
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

func (r *ReconcileMachinePool) syncClusterAutoscaler(
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
		&client.ListOptions{Raw: &metav1.ListOptions{TypeMeta: tm}},
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

func (r *ReconcileMachinePool) updatePoolStatusForMachineSets(
	pool *hivev1.MachinePool,
	machineSets []*machineapi.MachineSet,
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

	if (len(origPool.Status.MachineSets) == 0 && len(pool.Status.MachineSets) == 0) ||
		reflect.DeepEqual(origPool.Status, pool.Status) {
		return reconcile.Result{RequeueAfter: requeueAfter}, nil
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, errors.Wrap(r.Status().Update(context.Background(), pool), "failed to update pool status")
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
					Name:      os.Getenv(constants.HiveAWSServiceProviderCredentialsSecretRefEnvVar),
				},
				Role: cd.Spec.Platform.AWS.CredentialsAssumeRole,
			},
		}
		return NewAWSActuator(r.Client, creds, cd.Spec.Platform.AWS.Region, pool, masterMachine, r.scheme, logger)
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
		clusterVersion, err := getClusterVersion(cd)
		if err != nil {
			return nil, err
		}
		return NewGCPActuator(r.Client, creds, clusterVersion, masterMachine, remoteMachineSets, r.scheme, r.expectations, logger)
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
		return NewAzureActuator(creds, logger)
	case cd.Spec.Platform.OpenStack != nil:
		return NewOpenStackActuator(masterMachine, r.scheme, r.Client, logger)
	case cd.Spec.Platform.VSphere != nil:
		return NewVSphereActuator(masterMachine, r.scheme, logger)
	case cd.Spec.Platform.Ovirt != nil:
		return NewOvirtActuator(masterMachine, r.scheme, logger)
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

func getMinMaxReplicasForCAPIMachineSet(pool *hivev1.MachinePool, machineSets []*capiv1.MachineSet, machineSetIndex int) (min, max int32) {
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
	version, versionPresent := cd.Labels[constants.VersionMajorMinorPatchLabel]
	if !versionPresent {
		return "", errors.New("cluster version not set in clusterdeployment")
	}
	return version, nil
}

func platformAllowsZeroAutoscalingMinReplicas(cd *hivev1.ClusterDeployment) bool {
	// Since 4.5, AWS, Azure, and GCP allow zero-sized minReplicas for autoscaling
	if cd.Spec.Platform.AWS != nil || cd.Spec.Platform.Azure != nil || cd.Spec.Platform.GCP != nil {
		return true
	}

	// Since 4.7, OpenStack allows zero-sized minReplicas for autoscaling
	if cd.Spec.Platform.OpenStack != nil {
		majorMinorPatch, ok := cd.Labels[constants.VersionMajorMinorPatchLabel]
		if !ok {
			// can't determine whether to allow zero minReplicas
			return false
		}

		currentVersion, err := semver.Make(majorMinorPatch)
		if err != nil {
			// assume we can't set minReplicas to zero
			return false
		}

		minimumOpenStackVersion, err := semver.Make("4.7.0")
		if err != nil {
			// something terrible has happened
			return false
		}

		if currentVersion.GTE(minimumOpenStackVersion) {
			return true
		}

		return false
	}

	return false
}

// periodicSource uses the client to list the machinepools
// every duration (including some jitter) and creates a Generic
// event for each object.
// this is useful to create a steady stream of reconcile requests
// when some of the changes cannot be models in Watches.
type periodicSource struct {
	client   client.Client
	duration time.Duration

	logger log.FieldLogger
}

func newPeriodicSource(c client.Client, d time.Duration, logger log.FieldLogger) *periodicSource {
	return &periodicSource{
		client:   c,
		duration: d,
		logger:   logger,
	}
}

// Start is internal and should be called only by the Controller to register an EventHandler with the Informer
// to enqueue reconcile.Requests.
func (ps *periodicSource) Start(ctx context.Context,
	handler handler.EventHandler,
	queue workqueue.RateLimitingInterface,
	prcts ...predicate.Predicate) error {

	go wait.JitterUntilWithContext(ctx, ps.syncFunc(handler, queue, prcts...), ps.duration, 0.1, true)
	return nil
}

func (ps *periodicSource) syncFunc(handler handler.EventHandler,
	queue workqueue.RateLimitingInterface,
	prcts ...predicate.Predicate) func(context.Context) {

	return func(ctx context.Context) {
		mpList := &hivev1.MachinePoolList{}
		err := ps.client.List(ctx, mpList)
		if err != nil {
			ps.logger.WithError(err).Error("failed to list MachinePools")
			return
		}

		for idx := range mpList.Items {
			evt := event.GenericEvent{Object: &mpList.Items[idx]}

			shouldHandle := true
			for _, p := range prcts {
				if !p.Generic(evt) {
					shouldHandle = false
				}
			}

			if shouldHandle {
				handler.Generic(evt, queue)
			}
		}
	}
}

func (ps *periodicSource) String() string {
	return fmt.Sprintf("periodic source for MachinePools every %s", ps.duration)
}

package remotemachineset

import (
	"context"
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

	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"

	installertypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
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
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
	}
	r.actuatorBuilder = func(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error) {
		return r.createActuator(cd, remoteMachineSets, cdLog)
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
		log.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a.Object)
		return retval
	}

	pools := &hivev1.MachinePoolList{}
	err := r.List(context.TODO(), pools)
	if err != nil {
		// Could not list machine pools
		log.Errorf("Error listing machine pools. Value: %+v", a.Object)
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

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string, string) (client.Client, error)

	// actuatorBuilder is a function pointer to the function that builds the actuator
	actuatorBuilder func(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error)
}

// Reconcile reads that state of the cluster for a MachinePool object and makes changes to the
// remote cluster MachineSets based on the state read
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"machinePool": request.Name,
		"namespace":   request.Namespace,
	})
	cdLog.Info("reconciling machine pool")

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the MachinePool instance
	pool := &hivev1.MachinePool{}
	if err := r.Get(context.TODO(), request.NamespacedName, pool); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up machine pool")
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
		log.Debug("clusterdeployment does not exist")
		return r.removeFinalizer(pool)
	case err != nil:
		log.WithError(err).Error("error looking up cluster deploymnet")
		return reconcile.Result{}, err
	}

	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		log.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
		return reconcile.Result{}, nil
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return r.removeFinalizer(pool)
	}

	// If the cluster is unreachable, do not reconcile.
	if controllerutils.HasUnreachableCondition(cd) {
		cdLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		controllerutils.AddFinalizer(pool, finalizer)
		err := r.Update(context.Background(), pool)
		if err != nil {
			log.WithError(err).Log(controllerutils.LogLevel(err), "could not add finalizer")
		}
		return reconcile.Result{}, err
	}

	adminKubeconfigSecret := &corev1.Secret{}
	if err := r.Get(
		context.TODO(),
		types.NamespacedName{Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name, Namespace: cd.Namespace},
		adminKubeconfigSecret,
	); err != nil {
		cdLog.WithError(err).Error("unable to fetch admin kubeconfig secret")
		return reconcile.Result{}, err
	}
	kubeConfig, err := controllerutils.FixupKubeconfigSecretData(adminKubeconfigSecret.Data)
	if err != nil {
		cdLog.WithError(err).Error("unable to fixup admin kubeconfig")
		return reconcile.Result{}, err
	}

	remoteClusterAPIClient, err := r.remoteClusterAPIClientBuilder(string(kubeConfig), controllerName)
	if err != nil {
		cdLog.WithError(err).Error("error building remote cluster-api client connection")
		return reconcile.Result{}, err
	}

	if err := r.syncMachineSets(pool, cd, remoteClusterAPIClient, cdLog); err != nil {
		return reconcile.Result{}, err
	}

	if pool.DeletionTimestamp != nil {
		return r.removeFinalizer(pool)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRemoteMachineSet) syncMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClient client.Client,
	cdLog log.FieldLogger) error {

	cdLog.Info("reconciling machine pool for cluster deployment")

	// List MachineSets from remote cluster
	remoteMachineSets := &machineapi.MachineSetList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
	err := remoteClusterAPIClient.List(context.Background(), remoteMachineSets, client.UseListOptions(&client.ListOptions{
		Raw: &metav1.ListOptions{TypeMeta: tm},
	}))
	if err != nil {
		cdLog.WithError(err).Error("unable to fetch remote machine sets")
		return err
	}
	cdLog.Infof("found %v remote machine sets", len(remoteMachineSets.Items))

	generatedMachineSets := []*machineapi.MachineSet{}
	machineSetsToDelete := []*machineapi.MachineSet{}
	machineSetsToCreate := []*machineapi.MachineSet{}
	machineSetsToUpdate := []*machineapi.MachineSet{}

	if pool.DeletionTimestamp == nil {
		actuator, err := r.actuatorBuilder(cd, remoteMachineSets.Items, cdLog)
		if err != nil {
			cdLog.WithError(err).Error("unable to create actuator")
			return err
		}

		// Generate expected MachineSets for machine pool
		generatedMachineSets, err = r.generateMachineSetsForMachinePool(cd, pool, actuator, cdLog)
		if err != nil {
			cdLog.WithError(err).Error("unable to generate machine sets for machine pool")
			return err
		}

		cdLog.Infof("generated %v worker machine sets", len(generatedMachineSets))

		// Find MachineSets that need updating/creating
		for _, ms := range generatedMachineSets {
			found := false
			for _, rMS := range remoteMachineSets.Items {
				if ms.Name == rMS.Name {
					found = true
					objectModified := false
					objectMetaModified := false
					resourcemerge.EnsureObjectMeta(&objectMetaModified, &rMS.ObjectMeta, ms.ObjectMeta)
					msLog := cdLog.WithField("machineset", rMS.Name)

					if *rMS.Spec.Replicas != *ms.Spec.Replicas {
						msLog.WithFields(log.Fields{
							"desired":  *ms.Spec.Replicas,
							"observed": *rMS.Spec.Replicas,
						}).Info("replicas out of sync")
						rMS.Spec.Replicas = ms.Spec.Replicas
						objectModified = true
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
					break
				}
			}

			if !found {
				machineSetsToCreate = append(machineSetsToCreate, ms)
			}

		}
	}

	// Find MachineSets that need deleting
	for i, rMS := range remoteMachineSets.Items {
		if !isMachineSetControlledByMachinePool(cd, pool, &rMS) {
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
		cdLog.WithField("machineset", ms.Name).Info("creating machineset")
		err = remoteClusterAPIClient.Create(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to create machine set")
			return err
		}
	}

	for _, ms := range machineSetsToUpdate {
		cdLog.WithField("machineset", ms.Name).Info("updating machineset")
		err = remoteClusterAPIClient.Update(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to update machine set")
			return err
		}
	}

	for _, ms := range machineSetsToDelete {
		cdLog.WithField("machineset", ms.Name).Info("deleting machineset: ", ms)
		err = remoteClusterAPIClient.Delete(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to delete machine set")
			return err
		}
	}

	cdLog.Info("done reconciling machine sets for cluster deployment")
	return nil
}

// generateMachineSetsForMachinePool generates expected MachineSets for a machine pool
// using the installer MachineSets API for the MachinePool Platform.
func (r *ReconcileRemoteMachineSet) generateMachineSetsForMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, actuator Actuator, cdLog log.FieldLogger) ([]*machineapi.MachineSet, error) {
	// Generate expected MachineSets for Platform from InstallConfig
	machineSets, err := actuator.GenerateMachineSets(cd, pool, cdLog)
	if err != nil {
		return nil, errors.Wrap(err, "could not generate machinesets")
	}

	for _, ms := range machineSets {
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

	return machineSets, nil
}

func (r *ReconcileRemoteMachineSet) createActuator(cd *hivev1.ClusterDeployment, remoteMachineSets []machineapi.MachineSet, cdLog log.FieldLogger) (Actuator, error) {
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
		return NewAWSActuator(creds, cd.Spec.Platform.AWS.Region, remoteMachineSets, r.scheme, cdLog)
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
		return NewGCPActuator(creds, cdLog)
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

func isMachineSetControlledByMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, machineSet *machineapi.MachineSet) bool {
	return strings.HasPrefix(
		machineSet.Name,
		strings.Join([]string{cd.Spec.ClusterName, pool.Spec.Name, ""}, "-"),
	) ||
		machineSet.Labels[machinePoolNameLabel] == pool.Spec.Name
}

func (r *ReconcileRemoteMachineSet) removeFinalizer(pool *hivev1.MachinePool) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return reconcile.Result{}, nil
	}
	controllerutils.DeleteFinalizer(pool, finalizer)
	err := r.Status().Update(context.Background(), pool)
	if err != nil {
		log.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer")
	}
	return reconcile.Result{}, err
}

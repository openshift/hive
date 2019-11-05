package syncmachineset

import (
	"context"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	installtypes "github.com/openshift/installer/pkg/types"

	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	"github.com/openshift/hive/pkg/resource"
)

const (
	controllerName    = "syncmachineset"
	sshKeyName        = "ssh-publickey"
	gcpCredsSecretKey = "osServiceAccount.json"
)

// kubeCLIApplier knows how to ApplyRuntimeObject.
type kubeCLIApplier interface {
	ApplyRuntimeObject(obj runtime.Object, scheme *runtime.Scheme) (resource.ApplyResult, error)
}

// Add creates a new SyncMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	logger := log.WithField("controller", controllerName)
	return &ReconcileSyncMachineSet{
		Client:          controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		logger:          logger,
		scheme:          mgr.GetScheme(),
		actuatorBuilder: newMachineSetActuator,
		kubeCLI:         resource.NewHelperWithMetricsFromRESTConfig(mgr.GetConfig(), controllerName, logger),
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName+"-controller", mgr,
		controller.Options{Reconciler: r,
			MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileSyncMachineSet{}

// ReconcileSyncMachineSet reconciles the MachineSets defined in a ClusterDeployment object
type ReconcileSyncMachineSet struct {
	client.Client

	logger log.FieldLogger

	scheme *runtime.Scheme

	kubeCLI kubeCLIApplier

	actuatorBuilder func(client.Client, *hivev1.ClusterDeployment, log.FieldLogger) (Actuator, error)
}

// Reconcile reads the ClusterDeployment object and makes changes to the MachineSets that need to be
// synced to the remote cluster based on the values read from ClusterDeployment.Spec.Config.Machines
func (r *ReconcileSyncMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
	})
	cdLog.Info("reconciling cluster deployment")

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	if err := r.Get(context.TODO(), request.NamespacedName, cd); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("failed to look up cluster deployment")
		return reconcile.Result{}, err
	}
	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		log.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
		return reconcile.Result{}, nil
	}

	// Do not reconcile a ClusterDeployment being deleted.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Do not reconcile a ClusterDeployment for a platform that isn't supported
	if !isPlatformSupported(cd) {
		log.Debug("unsupported platform for machineSet syncing")
		return reconcile.Result{}, nil
	}

	err := r.syncMachineSets(cd, cdLog)

	return reconcile.Result{}, err
}

func (r *ReconcileSyncMachineSet) syncMachineSets(cd *hivev1.ClusterDeployment, logger log.FieldLogger) error {
	logger.Info("reconciling machine sets for cluster deployment")

	// Generate the machinesets we need to sync.
	machineSets, err := r.generateMachineSets(cd, logger)
	if err != nil {
		logger.WithError(err).Warn("failed to generate machinesets")
		return err
	}

	// Build the Syncset.
	syncSet, err := r.buildSyncSet(cd, machineSets, logger)
	if err != nil {
		return err
	}

	if _, err := r.kubeCLI.ApplyRuntimeObject(syncSet, r.scheme); err != nil {
		logger.WithError(err).Error("failed to apply syncset")
		return err
	}

	return nil
}

func (r *ReconcileSyncMachineSet) buildSyncSet(cd *hivev1.ClusterDeployment,
	machineSets []*machineapi.MachineSet,
	logger log.FieldLogger) (*hivev1.SyncSet, error) {

	rawList := make([]runtime.RawExtension, len(machineSets))

	for i := range machineSets {
		rawList[i] = runtime.RawExtension{Object: machineSets[i]}
	}

	ssName := apihelpers.GetResourceName(cd.Name, "machinesets")

	syncSet := &hivev1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName,
			Namespace: cd.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "SyncSet",
			APIVersion: hivev1.SchemeGroupVersion.String(),
		},
		Spec: hivev1.SyncSetSpec{
			SyncSetCommonSpec: hivev1.SyncSetCommonSpec{
				Resources:         rawList,
				ResourceApplyMode: "sync",
			},
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{
					Name: cd.Name,
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(cd, syncSet, r.scheme); err != nil {
		logger.WithError(err).Error("failed to set owner reference")
		return nil, err
	}

	return syncSet, nil
}

func (r *ReconcileSyncMachineSet) generateMachineSets(cd *hivev1.ClusterDeployment, logger log.FieldLogger) ([]*machineapi.MachineSet, error) {

	// Need a somewhat filled-out InstallConfig to have the OpenShift installer generate the MachineSets
	// from the MachinePools
	ic, err := r.generateInstallConfig(cd, logger)
	if err != nil {
		return nil, err
	}

	actuator, err := r.actuatorBuilder(r.Client, cd, logger)
	if err != nil {
		return nil, err
	}

	return actuator.GenerateMachineSets(cd, ic, logger)
}

func (r *ReconcileSyncMachineSet) generateInstallConfig(cd *hivev1.ClusterDeployment, logger log.FieldLogger) (*installtypes.InstallConfig, error) {
	sshKey, err := controllerutils.LoadSecretData(r, cd.Spec.SSHKey.Name, cd.Namespace, sshKeyName)
	if err != nil {
		logger.WithError(err).Error("failed to load ssh key for ClusterDeployment")
		return nil, err
	}

	ic, err := install.GenerateInstallConfig(cd, sshKey, "{}", false)
	if err != nil {
		logger.WithError(err).Error("failed to generate install config")
		return nil, err
	}

	return ic, nil
}

func newMachineSetActuator(kubeClient client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (Actuator, error) {

	if cd.Spec.Platform.GCP != nil {
		return getGCPActuator(kubeClient, cd, logger)
	}

	return nil, errors.New("cannot create actuator for unsupported platform")
}

func getGCPActuator(kubeClient client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (Actuator, error) {
	gcpCredsSecret, err := getGCPClientCredentials(kubeClient, cd, logger)
	if err != nil {
		return nil, err
	}

	gcpCreds := gcpCredsSecret.Data[gcpCredsSecretKey]
	actuator, err := NewGCPActuator(gcpCreds, cd.Spec.Platform.GCP.ProjectID, logger)
	if err != nil {
		logger.WithError(err).Warn("failed to create GCP actuator")
	}
	return actuator, err
}

func getGCPClientCredentials(kubeClient client.Client, cd *hivev1.ClusterDeployment, logger log.FieldLogger) (*corev1.Secret, error) {
	gcpCreds := &corev1.Secret{}
	if cd.Spec.PlatformSecrets.GCP == nil {
		return nil, errors.New("missing GCP platform secrets in clusterDeployment")
	}
	credsKey := types.NamespacedName{Name: cd.Spec.PlatformSecrets.GCP.Credentials.Name,
		Namespace: cd.Namespace}
	if err := kubeClient.Get(context.TODO(), credsKey, gcpCreds); err != nil {
		logger.WithError(err).Warn("failed to read secret with GCP creds")
		return nil, err
	}

	return gcpCreds, nil
}

func isPlatformSupported(cd *hivev1.ClusterDeployment) bool {
	return cd.Spec.Platform.GCP != nil
}

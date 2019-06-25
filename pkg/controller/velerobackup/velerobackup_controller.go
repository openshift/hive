package velerobackup

import (
	"context"
	"fmt"
	"time"

	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	controllerName = "velerobackup"
)

var (
	// DefaultExcludedBackupResources is the deault list of excludes when backing up resources.
	DefaultExcludedBackupResources = []string{
		// NOTE: even though pods is in the "core" group, specifying "pods.core" here will cause
		// pods to BE BACKED UP (not excluded)!!!
		"pods",
		"jobs.batch",
	}
)

// Add creates a new Backup Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBackup{
		Client:       controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:       mgr.GetScheme(),
		logger:       log.WithField("controller", controllerName),
		checksumFunc: controllerutils.GetChecksumOfObjects,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName+"-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	reconciler := r.(*ReconcileBackup)
	err = reconciler.registerClusterDeploymentWatch(c)
	if err != nil {
		return err
	}

	return nil // If we got here, then no errors occurred.
}

// This ensures that ReconcileBackup struct implements all functions that the reconcile.Reconciler interface requires.
var _ reconcile.Reconciler = &ReconcileBackup{}

// ReconcileBackup ensures that Velero backup objects are created when changes are made to Hive objects.
type ReconcileBackup struct {
	client.Client
	scheme *runtime.Scheme

	logger       log.FieldLogger
	checksumFunc controllerutils.ChecksumOfObjectsFunc
}

// Reconcile ensures that all Hive object changes have corresponding Velero backup objects.
// ClusterDeployment.Spec.Config.Machines
// +kubebuilder:rbac:groups=hive.openshift.io,resources=clusterdeployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=create
func (r *ReconcileBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	rrLogger := addReconcileRequestLoggerFields(r.logger, request)

	// For logging, we need to see when the reconciliation loop starts and ends.
	rrLogger.Info("reconciling backups and Hive object changes")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		rrLogger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Get all ClusterDeployments that have a different hash than the last backup hash.
	changedCDs, err := r.getModifiedClusterDeploymentsInNamespace(request.Namespace)
	if err != nil {
		rrLogger.WithError(err).Error("error finding changed cluster deployments in namespace")
		return reconcile.Result{}, err
	}

	if len(changedCDs) == 0 {
		rrLogger.Debug("Nothing changed, so nothing to back up. Don't create a Velero backup object.")
		return reconcile.Result{}, nil
	}

	// There are changes that need to be backed up.
	err = createVeleroBackupObject(r.Client, request.Namespace)
	if err != nil {
		rrLogger.WithError(err).Error("error creating velero backup object")
		return reconcile.Result{}, err
	}

	// If the above is successful, save this objects new checksum to the "lastBackupChecksum" annotation.
	err = r.updateClusterDeploymentLastBackupChecksum(changedCDs)
	if err != nil {
		rrLogger.WithError(err).Errorf("error updating %v annotation", controllerutils.LastBackupAnnotation)
		// Not returning with an error because a backup object was created,
		// we just failed to update the objects with the backup checksum (not a fatal error).
		// This will cause these objects to be backed during the next change in this ns or the next reconcile.
	}

	return reconcile.Result{}, nil
}

func addReconcileRequestLoggerFields(logger log.FieldLogger, request reconcile.Request) *log.Entry {
	return logger.WithFields(log.Fields{
		"NamespacedName": request.NamespacedName,
	})
}

// createVeleroBackupObjectForNamespace creates a Velero Backup object for the namespace specified.
// The Backup options are set specifically for Hive object backups.
// DO NOT use this function call for any other type objects as it may not back them up correctly.
func createVeleroBackupObject(client client.Client, namespace string) error {
	formatStr := "2006-01-02t15-04-05z"
	timestamp := time.Now().UTC().Format(formatStr)

	backupName := fmt.Sprintf("backup-%v-%v", namespace, timestamp)
	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: velerov1.DefaultNamespace,
		},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{
				namespace,
			},
			ExcludedResources: DefaultExcludedBackupResources,
		},
	}

	return client.Create(context.TODO(), backup)
}

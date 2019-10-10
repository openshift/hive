package velerobackup

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"

	hiveconstants "github.com/openshift/hive/pkg/constants"
)

const (
	controllerName = "velerobackup"
	errChecksum    = "HIVE_CHECKSUM_ERR_97A29D08"

	// Default reconcile rate limiting to 1 backup every 3 minutes.
	defaultReconcileRateLimitDuration = 3 * time.Minute
)

var (
	// defaultExcludedBackupResources is the default list of excludes when backing up resources.
	defaultExcludedBackupResources = []string{
		// NOTE: even though pods is in the "core" group, specifying "pods.core" here will cause
		// pods to BE BACKED UP (not excluded)!!!
		"pods",
		"jobs.batch",
		"checkpoints.hive.openshift.io",
	}

	hiveNamespaceScopedTypesToWatch = []runtime.Object{
		&hivev1.ClusterDeployment{},
		&hivev1.SyncSet{},
		&hivev1.DNSZone{},
	}

	hiveNamespaceScopedListTypes = []runtime.Object{
		&hivev1.ClusterDeploymentList{},
		&hivev1.SyncSetList{},
		&hivev1.DNSZoneList{},
	}
)

// Add creates a new Backup Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	reconciler, err := NewReconciler(mgr)
	if err != nil {
		return err
	}

	return AddToManager(mgr, reconciler)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) (reconcile.Reconciler, error) {
	logger := log.WithField("controller", controllerName)
	reconcileRateLimitDuration := defaultReconcileRateLimitDuration
	minBackupPeriodSecondsStr := os.Getenv(hiveconstants.MinBackupPeriodSecondsEnvVar)
	if minBackupPeriodSecondsStr != "" {
		// The environment variable has been specified.
		minBackupPeriodSeconds, err := strconv.Atoi(minBackupPeriodSecondsStr)
		if err != nil {
			logger.WithError(err).Errorf("Couldn't parse environment variable %v: %v", hiveconstants.MinBackupPeriodSecondsEnvVar, minBackupPeriodSecondsStr)
			return nil, err
		}
		reconcileRateLimitDuration = time.Duration(minBackupPeriodSeconds) * time.Second
	}

	return &ReconcileBackup{
		Client:                     controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                     mgr.GetScheme(),
		reconcileRateLimitDuration: reconcileRateLimitDuration,
		logger:                     logger,
	}, nil
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName+"-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	reconciler := r.(*ReconcileBackup)
	return reconciler.registerHiveObjectWatches(c)
}

func (r *ReconcileBackup) registerHiveObjectWatches(c controller.Controller) error {
	for _, t := range hiveNamespaceScopedTypesToWatch {
		if err := c.Watch(&source.Kind{Type: t.DeepCopyObject()}, &handler.EnqueueRequestsFromMapFunc{
			// Queue up the NS for this Hive Object
			ToRequests: handler.ToRequestsFunc(func(a handler.MapObject) []reconcile.Request {
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: a.Meta.GetNamespace()}}}
			}),
		}); err != nil {
			return err
		}
	}
	return nil
}

// This ensures that ReconcileBackup struct implements all functions that the reconcile.Reconciler interface requires.
var _ reconcile.Reconciler = &ReconcileBackup{}

// ReconcileBackup ensures that Velero backup objects are created when changes are made to Hive objects.
type ReconcileBackup struct {
	client.Client
	reconcileRateLimitDuration time.Duration
	scheme                     *runtime.Scheme

	logger log.FieldLogger
}

// Reconcile ensures that all Hive object changes have corresponding Velero backup objects.
func (r *ReconcileBackup) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	nsLogger := r.logger.WithField("namespace", request.Namespace)

	// For logging, we need to see when the reconciliation loop starts and ends.
	nsLogger.Info("reconciling backups and Hive object changes")
	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		nsLogger.WithField("elapsed", dur).Info("reconcile complete")
	}()

	cp, checkpointFound, err := r.getNamespaceCheckpoint(request.Namespace, nsLogger)
	if err != nil {
		nsLogger.WithError(err).Error("error getting namespace CheckPoint")
		return reconcile.Result{}, err
	}

	// Only rate limit AFTER the first checkpoint has been created.
	if checkpointFound {
		// Check to see how long since the last back was taken (we may need to rate limit)
		timeSinceLastBackup := time.Since(cp.Spec.LastBackupTime.Time)
		if timeSinceLastBackup < r.reconcileRateLimitDuration {
			// calculate the next time this reconcile should attempt to run.
			requestAfter := (r.reconcileRateLimitDuration - timeSinceLastBackup)

			nsLogger.Infof("Rate limiting this reconcile. Will reconcile this namespace again in %v", requestAfter)

			// Requeue this reconcile because we've already taken a backup within the rate limit duration.
			return reconcile.Result{
				RequeueAfter: requestAfter,
			}, nil
		}
	}

	objects, err := r.getRuntimeObjects(hiveNamespaceScopedListTypes, request.Namespace)
	if err != nil {
		nsLogger.WithError(err).Error("Failed to list hive objects in namespace.")
		return reconcile.Result{}, err
	}

	currentChecksum := r.calculateObjectsChecksumWithoutStatus(nsLogger, objects...)

	// See if anything has changed.
	if cp.Spec.LastBackupChecksum == currentChecksum {
		nsLogger.Debug("Nothing changed, so nothing to back up. Don't create a Velero backup object.")
		return reconcile.Result{}, nil
	}

	// There are changes that need to be backed up.
	timestamp := metav1.Now()
	backupRef, err := r.createVeleroBackupObject(request.Namespace, timestamp)
	if err != nil {
		nsLogger.WithError(err).Error("error creating velero backup object")
		return reconcile.Result{}, err
	}

	// If the above is successful, save this object's new checksum to the CheckPoint object for the namespace.
	cp.Spec.LastBackupChecksum = currentChecksum
	cp.Spec.LastBackupTime = timestamp
	cp.Spec.LastBackupRef = backupRef
	err = r.createOrUpdateNamespaceCheckpoint(cp, checkpointFound, nsLogger)
	if err != nil {
		nsLogger.WithError(err).Log(controllerutils.LogLevel(err), "error updating namespace CheckPoint.")
		// Not returning with an error because a backup object was created,
		// we just failed to update the backup checksum in the CheckPoint object (not a fatal error).
		// This will cause these objects to be backed up during the next change in this ns or the next reconcile.
	}

	return reconcile.Result{}, nil
}

// createVeleroBackupObjectForNamespace creates a Velero Backup object for the namespace specified.
// The Backup options are set specifically for Hive object backups.
// DO NOT use this function call for any other type objects as it may not back them up correctly.
func (r *ReconcileBackup) createVeleroBackupObject(namespace string, t metav1.Time) (corev1.ObjectReference, error) {
	formatStr := "2006-01-02t15-04-05z"
	timestamp := t.UTC().Format(formatStr)

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("backup-%v-%v", namespace, timestamp),
			Namespace: velerov1.DefaultNamespace,
		},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{
				namespace,
			},
			ExcludedResources: defaultExcludedBackupResources,
		},
	}

	backupRef := corev1.ObjectReference{
		Name:      backup.Name,
		Namespace: backup.Namespace,
	}

	return backupRef, r.Create(context.TODO(), backup)
}

func (r *ReconcileBackup) getRuntimeObjects(typesToList []runtime.Object, namespace string) ([]runtime.Object, error) {
	nsObjects := []runtime.Object{}

	for _, t := range typesToList {
		listObj := t.DeepCopyObject()
		if err := r.List(context.TODO(), listObj, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		list, err := meta.ExtractList(listObj)
		if err != nil {
			return nil, err
		}

		nsObjects = append(nsObjects, list...)
	}

	return nsObjects, nil
}

func (r *ReconcileBackup) getNamespaceCheckpoint(namespace string, logger log.FieldLogger) (*hivev1.Checkpoint, bool, error) {
	cp := &hivev1.Checkpoint{}
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: namespace, Name: hiveconstants.CheckpointName}, cp)
	found := true
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("First time backing up this namespace, creating CheckPoint object, ")
			cp.Name = hiveconstants.CheckpointName
			cp.Namespace = namespace
			found = false
		} else {
			logger.WithError(err).Error("Failed getting CheckPoint object for namespace.")
			return nil, false, err
		}
	}

	return cp, found, nil
}

func (r *ReconcileBackup) createOrUpdateNamespaceCheckpoint(cp *hivev1.Checkpoint, found bool, logger log.FieldLogger) error {
	var err error

	if found {
		err = r.Update(context.TODO(), cp)
	} else {
		err = r.Create(context.TODO(), cp)
	}

	if err != nil {
		logger.WithError(err).Log(controllerutils.LogLevel(err), "Failed to create or update CheckPoint object in namespace.")
	}

	return err
}

// calculateChecksum computes the runtime object's hash WITHOUT the status and ResourceVersion.
// Status is not used to determine if the object has changed and ResourceVersion changes each time status is updated.
// We never return an error from this function as that could lead to a
// situation where other objects change, but because one of the objects
// checksum errored, then the other objects won't get backed up.
// Instead for objects that error, we return a constant checksum value.
// This function should rarely, if ever, return an error.
func (r *ReconcileBackup) calculateObjectsChecksumWithoutStatus(logger log.FieldLogger, objects ...runtime.Object) string {
	checksums := make([]string, len(objects))

	for i, object := range objects {
		var meta *metav1.ObjectMeta
		var spec interface{}

		switch t := object.(type) {
		case *hivev1.ClusterDeployment:
			meta = &t.ObjectMeta
			spec = &t.Spec
		case *hivev1.SyncSet:
			meta = &t.ObjectMeta
			spec = &t.Spec
		case *hivev1.DNSZone:
			meta = &t.ObjectMeta
			spec = &t.Spec
		default:
			logger.Warningf("Unknown Type: %T", object)
			checksums[i] = errChecksum
			continue
		}

		// We need to take the checksum WITHOUT the previous ResourceVersion
		// as when we update the checksum annotation, ResourceVersion changes.
		metaCopy := meta.DeepCopy()
		metaCopy.ResourceVersion = ""

		checksum, err := controllerutils.GetChecksumOfObjects(metaCopy, spec)
		if err != nil {
			logger.WithError(err).Info("error calculating object checksum")
			checksum = errChecksum
		}

		checksums[i] = checksum
	}

	// This ensures that for the same set of objects we generate the same combined checksum.
	// Otherwise a different order could cause the checksum to be different every time.
	sort.Strings(checksums)

	// Specifically using GetChecksumOfObject since checksums is a slice of string and using
	// GetChecksumOfObjects would give the checksum of nested slice in slice of string.
	combinedChecksum, err := controllerutils.GetChecksumOfObject(checksums)
	if err != nil {
		logger.WithError(err).Info("error calculating hive objects' combined checksum")
		return errChecksum
	}

	return combinedChecksum
}

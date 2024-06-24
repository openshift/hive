package privatelink

import (
	"context"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/privatelink/actuator"
	"github.com/openshift/hive/pkg/controller/privatelink/conditions"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

// PrivateLink is the PrivateLink to be reconciled by the PrivateLinkReconciler
type PrivateLink struct {
	client.Client

	cd *hivev1.ClusterDeployment

	hubActuator actuator.Actuator

	linkActuator actuator.Actuator

	logger log.FieldLogger
}

func (pl *PrivateLink) Reconcile(privateLinkEnabled bool) (reconcile.Result, error) {

	if paused, err := strconv.ParseBool(pl.cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		pl.logger.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	// Initialize cluster deployment conditions if not present
	newConditions, changed := conditions.InitializeConditions(pl.cd)
	if changed {
		pl.cd.Status.Conditions = newConditions
		pl.logger.Info("initializing private link controller conditions")
		if err := pl.Status().Update(context.TODO(), pl.cd); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update cluster deployment status")
		}
		return reconcile.Result{}, nil
	}

	if !privateLinkEnabled {
		if pl.cleanupRequired() {
			// private link was disabled for this cluster so cleanup is required.
			return pl.cleanupClusterDeployment()
		}

		pl.logger.Debug("cluster deployment does not have private link enabled, so skipping")
		return reconcile.Result{}, nil
	}

	if pl.cd.DeletionTimestamp != nil {
		return pl.cleanupClusterDeployment()
	}

	// Add finalizer if not already present
	if !controllerutils.HasFinalizer(pl.cd, finalizer) {
		pl.logger.Debug("adding finalizer to ClusterDeployment")
		controllerutils.AddFinalizer(pl.cd, finalizer)
		if err := pl.Update(context.Background(), pl.cd); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "error adding finalizer to ClusterDeployment")
		}
	}

	// See if we need to sync. This is what rate limits our cloud API usage, but allows for immediate syncing
	// on changes and deletes.
	shouldSync, syncAfter := pl.shouldSync()
	if !shouldSync {
		pl.logger.WithFields(log.Fields{
			"syncAfter": syncAfter,
		}).Debug("Sync not needed")

		return reconcile.Result{RequeueAfter: syncAfter}, nil
	}

	if pl.cd.Spec.Installed {
		pl.logger.Debug("reconciling already installed cluster deployment")
		return pl.reconcilePrivateLink(pl.cd.Spec.ClusterMetadata)
	}

	if pl.cd.Status.ProvisionRef == nil {
		pl.logger.Debug("waiting for cluster deployment provision to start, will retry soon.")
		return reconcile.Result{}, nil
	}

	cpLog := pl.logger.WithField("provision", pl.cd.Status.ProvisionRef.Name)
	cp := &hivev1.ClusterProvision{}
	err := pl.Get(context.TODO(), types.NamespacedName{Name: pl.cd.Status.ProvisionRef.Name, Namespace: pl.cd.Namespace}, cp)
	if apierrors.IsNotFound(err) {
		cpLog.Warn("linked cluster provision not found")
		return reconcile.Result{}, err
	}
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not get provision")
	}

	if cp.Spec.PrevInfraID != nil && *cp.Spec.PrevInfraID != "" && pl.cleanupRequired() {
		lastCleanup := pl.cd.Annotations[lastCleanupAnnotationKey]
		if lastCleanup != *cp.Spec.PrevInfraID {
			pl.logger.WithField("prevInfraID", *cp.Spec.PrevInfraID).
				Info("cleaning up PrivateLink resources from previous attempt")

			if err := pl.cleanupPreviousProvisionAttempt(cp); err != nil {
				pl.logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")

				if err := conditions.SetErrConditionWithRetry(pl.Client, pl.cd, "CleanupForProvisionReattemptFailed", err, pl.logger); err != nil {
					return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
				}
				return reconcile.Result{}, err
			}

			if err := conditions.SetReadyConditionWithRetry(pl.Client, pl.cd, corev1.ConditionFalse,
				"PreviousAttemptCleanupComplete",
				"successfully cleaned up resources from previous provision attempt so that next attempt can start",
				pl.logger); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
			}

			return reconcile.Result{Requeue: true}, nil
		}
	}

	if cp.Spec.InfraID == nil ||
		(cp.Spec.InfraID != nil && *cp.Spec.InfraID == "") ||
		(cp.Spec.AdminKubeconfigSecretRef == nil) ||
		(cp.Spec.AdminKubeconfigSecretRef != nil && cp.Spec.AdminKubeconfigSecretRef.Name == "") {
		pl.logger.Debug("waiting for cluster deployment provision to provide ClusterMetadata, will retry soon.")
		return reconcile.Result{}, nil
	}

	return pl.reconcilePrivateLink(&hivev1.ClusterMetadata{InfraID: *cp.Spec.InfraID, AdminKubeconfigSecretRef: *cp.Spec.AdminKubeconfigSecretRef})
}

func (pl *PrivateLink) cleanupRequired() bool {
	return pl.hubActuator.CleanupRequired(pl.cd) ||
		pl.linkActuator.CleanupRequired(pl.cd)
}

func (pl *PrivateLink) cleanupClusterDeployment() (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(pl.cd, finalizer) {
		return reconcile.Result{}, nil
	}

	if pl.cd.Spec.ClusterMetadata != nil && pl.cleanupRequired() {
		if err := pl.cleanupPrivateLink(pl.cd.Spec.ClusterMetadata); err != nil {
			pl.logger.WithError(err).Error("error cleaning up PrivateLink resources for ClusterDeployment")

			if err := conditions.SetErrConditionWithRetry(pl.Client, pl.cd, "CleanupForDeprovisionFailed", err, pl.logger); err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
			}
			return reconcile.Result{}, err
		}

		if err := conditions.SetReadyConditionWithRetry(pl.Client, pl.cd, corev1.ConditionFalse,
			"DeprovisionCleanupComplete",
			"successfully cleaned up private link resources created to deprovision cluster",
			pl.logger); err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
		}
	}

	pl.logger.Info("removing finalizer from ClusterDeployment")
	controllerutils.DeleteFinalizer(pl.cd, finalizer)
	if err := pl.Update(context.Background(), pl.cd); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "could not remove finalizer from ClusterDeployment")
	}

	return reconcile.Result{}, nil
}

func (pl *PrivateLink) cleanupPrivateLink(metadata *hivev1.ClusterMetadata) error {
	if err := pl.hubActuator.Cleanup(pl.cd, metadata, pl.logger); err != nil {
		return err
	}

	if err := pl.linkActuator.Cleanup(pl.cd, metadata, pl.logger); err != nil {
		return err
	}

	return nil
}

func (pl *PrivateLink) reconcilePrivateLink(metadata *hivev1.ClusterMetadata) (reconcile.Result, error) {
	dnsRecord := &actuator.DnsRecord{}

	linkResult, err := pl.linkActuator.Reconcile(pl.cd, metadata, dnsRecord, pl.logger)
	if err != nil || !linkResult.IsZero() {
		return linkResult, err
	}

	hubResult, err := pl.hubActuator.Reconcile(pl.cd, metadata, dnsRecord, pl.logger)
	if err != nil || !hubResult.IsZero() {
		return hubResult, err
	}

	if err := conditions.SetReadyConditionWithRetry(pl.Client, pl.cd, corev1.ConditionTrue,
		"PrivateLinkAccessReady",
		"private link access is ready for use",
		pl.logger); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to update condition on cluster deployment")
	}

	return reconcile.Result{}, nil
}

func (pl *PrivateLink) cleanupPreviousProvisionAttempt(cp *hivev1.ClusterProvision) error {
	if pl.cd.Spec.ClusterMetadata == nil {
		return errors.New("cannot cleanup previous resources because the admin kubeconfig is not available")
	}
	metadata := &hivev1.ClusterMetadata{
		InfraID:                  *cp.Spec.PrevInfraID,
		AdminKubeconfigSecretRef: pl.cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef,
	}

	if err := pl.cleanupPrivateLink(metadata); err != nil {
		return errors.Wrap(err, "error cleaning up PrivateLink resources for ClusterDeployment")
	}
	if pl.cd.Annotations == nil {
		pl.cd.Annotations = map[string]string{}
	}
	pl.cd.Annotations[lastCleanupAnnotationKey] = metadata.InfraID
	return updateAnnotations(pl.Client, pl.cd)
}

// shouldSync returns if we should sync the ClusterDeployment. If it returns false, it also returns
// the duration after which we should try to check if sync is required.
func (pl *PrivateLink) shouldSync() (bool, time.Duration) {
	window := 2 * time.Hour
	if pl.cd.DeletionTimestamp != nil && !controllerutils.HasFinalizer(pl.cd, finalizer) {
		return false, 0 // No finalizer means our cleanup has been completed. There's nothing left to do.
	}

	if pl.cd.DeletionTimestamp != nil {
		return true, 0 // We're in a deleting state, sync now.
	}

	failedCondition := controllerutils.FindCondition(pl.cd.Status.Conditions, hivev1.PrivateLinkFailedClusterDeploymentCondition)
	if failedCondition != nil && failedCondition.Status == corev1.ConditionTrue {
		return true, 0 // we have failed to reconcile and therefore should continue to retry for quick recovery
	}

	readyCondition := controllerutils.FindCondition(pl.cd.Status.Conditions, hivev1.PrivateLinkReadyClusterDeploymentCondition)
	if readyCondition == nil || readyCondition.Status != corev1.ConditionTrue {
		return true, 0 // we have not reached Ready level
	}
	delta := time.Since(readyCondition.LastProbeTime.Time)

	if !pl.cd.Spec.Installed {
		// as cluster is installing, but the private link has been setup once, we wait
		// for a shorter duration before reconciling again.
		window = 10 * time.Minute
	}

	if delta >= window {
		// We haven't sync'd in over resync duration time, sync now.
		return true, 0
	}

	syncAfter := (window - delta).Round(time.Minute)
	if syncAfter == 0 {
		// if it is less than a minute, sync after a minute
		syncAfter = time.Minute
	}

	if pl.linkActuator.ShouldSync(pl.cd) ||
		pl.hubActuator.ShouldSync(pl.cd) {
		// if there are changes to be made
		return true, 0
	}

	// We didn't meet any of the criteria above, so we should not sync.
	return false, syncAfter
}

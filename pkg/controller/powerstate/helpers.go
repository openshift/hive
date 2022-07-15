package powerstate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hiveintv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	// hibernateAfterSyncSetsNotApplied is the amount of time to wait
	// before hibernating when SyncSets have not been applied
	hibernateAfterSyncSetsNotApplied = 10 * time.Minute
)

// syncSetsApplied answers whether this cd had syncsets successfully applied after initial installation.
// If it has not, the second return indicates how long we should wait before giving up on it and proceeding
// with hibernation.
// If the cluster isn't installed yet, this defaults to hibernateAfterSyncSetsNotApplied, since it will be
// at least that long before we need to time out. (If they are successfully applied in the meantime, the cd
// will be enqueued immediately anyway.)
func (r *powerStateReconciler) syncSetsApplied(cd *hivev1.ClusterDeployment) (bool, time.Duration, error) {
	if cd.Status.InstalledTimestamp == nil {
		return false, hibernateAfterSyncSetsNotApplied, nil
	}
	if r.clusterSync == nil {
		clusterSync := &hiveintv1alpha1.ClusterSync{}
		if err := r.Get(context.Background(), types.NamespacedName{Namespace: cd.Namespace, Name: cd.Name}, clusterSync); err != nil {
			// This may be NotFound, which means the clustersync controller hasn't created the ClusterSync yet.
			return false, 0, fmt.Errorf("could not get ClusterSync: %v", err)
		}
		r.clusterSync = clusterSync
	}
	if r.clusterSync.Status.FirstSuccessTime != nil {
		return true, 0, nil
	}
	remaining := hibernateAfterSyncSetsNotApplied - time.Since(cd.Status.InstalledTimestamp.Time)
	if remaining < 0 {
		remaining = 0
	}
	return false, remaining, nil
}

// timeUntilHibernateAfter computes the duration until we should flip spec.powerState to Hibernating.
// If that's n/a -- e.g. the desired state is already hibernating, or hibernateAfter isn't set -- it returns nil.
func (r *powerStateReconciler) timeUntilHibernateAfter(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) *time.Duration {
	if cd.Spec.HibernateAfter == nil {
		return nil
	}
	if cd.Spec.PowerState == hivev1.ClusterPowerStateHibernating {
		return nil
	}

	// Pool clusters wait until they're claimed for HibernateAfter to have effect.
	poolRef := cd.Spec.ClusterPoolRef
	isUnclaimedPoolCluster := poolRef != nil && poolRef.PoolName != "" &&
		// Upgrade note: If we hit this code path on a CD that was claimed before upgrading to
		// where we introduced ClaimedTimestamp, then that CD was Hibernating when it was claimed
		// (because that's the same time we introduced ClusterPool.RunningCount) so it's safe to
		// just use installed/last-resumed as the baseline for hibernateAfter.
		(poolRef.ClaimName == "" || poolRef.ClaimedTimestamp == nil)
	if isUnclaimedPoolCluster {
		return nil
	}

	// As the baseline timestamp for determining whether HibernateAfter should trigger, we use the latest of:
	// - When the cluster finished installing (status.installedTimestamp)
	// - When the cluster was claimed (spec.clusterPoolRef.claimedTimestamp -- ClusterPool CDs only)
	// - The last time the cluster resumed (status.conditions[Hibernating].lastTransitionTime if not hibernating (but see TODO))
	hibernateAfterDur := cd.Spec.HibernateAfter.Duration
	hibLog := cdLog.WithField("hibernateAfter", hibernateAfterDur)

	// The nil values of each of these will make them "earliest" and therefore unused.
	var installedSince, runningSince, claimedSince time.Time
	var isRunning bool

	installedSince = cd.Status.InstalledTimestamp.Time
	hibLog = hibLog.WithField("installedSince", installedSince)

	if poolRef != nil && poolRef.ClaimedTimestamp != nil {
		claimedSince = poolRef.ClaimedTimestamp.Time
		hibLog = hibLog.WithField("claimedSince", claimedSince)
	} else {
		// This means it's not a pool cluster (!isUnclaimedPoolCluster && poolRef == nil)
		hibLog.Debug("cluster does not belong to a clusterpool")
	}

	if hc := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterHibernatingCondition); hc.Status == corev1.ConditionUnknown {
		hibLog.Debug("cluster has never been hibernated")
		isRunning = true
	}
	if rc := controllerutils.FindCondition(cd.Status.Conditions, hivev1.ClusterReadyCondition); rc.Status == corev1.ConditionTrue {
		runningSince = rc.LastTransitionTime.Time
		hibLog = hibLog.WithField("runningSince", runningSince)
		isRunning = true
	}

	if !isRunning {
		return nil
	}

	// Which timestamp should we use to calculate HibernateAfter from?
	// Sort our timestamps in descending order and use the first (latest) one.
	stamps := []time.Time{installedSince, claimedSince, runningSince}
	sort.Slice(stamps,
		func(i, j int) bool {
			return stamps[i].After(stamps[j])
		},
	)
	expiry := stamps[0].Add(hibernateAfterDur)
	hibLog.Debugf("cluster should be hibernating after: %s", expiry)
	timeUntilHibernateAfter := time.Until(expiry)
	return &timeUntilHibernateAfter
}

func (r *powerStateReconciler) setHibernatingStatus(cd *hivev1.ClusterDeployment, hibCondMsg string, cdLog log.FieldLogger) error {
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonHibernating,
		hibCondMsg, corev1.ConditionTrue, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStoppingOrHibernating,
		clusterHibernatingMsg, corev1.ConditionFalse, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateHibernating
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateHibernating
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

func (r *powerStateReconciler) setWaitingForMachinesToStopStatus(cd *hivev1.ClusterDeployment, remaining []string, cdLog log.FieldLogger) error {
	sort.Strings(remaining) // we want to make sure the message is stable.
	msg := fmt.Sprintf("Stopping cluster machines. Some machines have not yet stopped: %s", strings.Join(remaining, ","))
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonWaitingForMachinesToStop, msg,
		corev1.ConditionFalse, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStoppingOrHibernating,
		clusterHibernatingMsg, corev1.ConditionFalse, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateWaitingForMachinesToStop
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForMachinesToStop
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

func (r *powerStateReconciler) setWaitingForMachinesStatus(cd *hivev1.ClusterDeployment, remaining []string, cdLog log.FieldLogger) error {
	sort.Strings(remaining) // we want to make sure the message is stable.
	msg := fmt.Sprintf("Waiting for cluster machines to start. Some machines are not yet running: %s (step 1/4)", strings.Join(remaining, ","))
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning,
		clusterResumingOrRunningMsg, corev1.ConditionFalse, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonWaitingForMachines, msg,
		corev1.ConditionFalse, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateWaitingForMachines
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateWaitingForMachines
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

func (r *powerStateReconciler) setRunningStatus(cd *hivev1.ClusterDeployment, runCondMsg string, cdLog log.FieldLogger) error {
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning,
		clusterResumingOrRunningMsg, corev1.ConditionFalse, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonRunning, runCondMsg,
		corev1.ConditionTrue, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateRunning
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateRunning
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

func (r *powerStateReconciler) setFailedToStopMachinesStatus(cd *hivev1.ClusterDeployment, errIn error, cdLog log.FieldLogger) error {
	msg := fmt.Sprintf("Failed to stop machines: %v", errIn)
	cdLog.Error(msg)
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonFailedToStop, msg,
		corev1.ConditionFalse, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonStoppingOrHibernating,
		clusterHibernatingMsg, corev1.ConditionFalse, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateFailedToStop
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateFailedToStop
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

func (r *powerStateReconciler) setFailedToStartMachinesStatus(cd *hivev1.ClusterDeployment, errIn error, cdLog log.FieldLogger) error {
	msg := fmt.Sprintf("Failed to start machines: %v", errIn)
	cdLog.Error(msg)
	changed1 := r.setCDCondition(cd, hivev1.ClusterHibernatingCondition, hivev1.HibernatingReasonResumingOrRunning,
		clusterResumingOrRunningMsg, corev1.ConditionFalse, cdLog)
	changed2 := r.setCDCondition(cd, hivev1.ClusterReadyCondition, hivev1.ReadyReasonFailedToStartMachines, msg,
		corev1.ConditionFalse, cdLog)
	changed3 := cd.Status.PowerState != hivev1.ClusterPowerStateFailedToStartMachines
	if changed1 || changed2 || changed3 {
		cd.Status.PowerState = hivev1.ClusterPowerStateFailedToStartMachines
		return r.updateClusterDeploymentStatus(cd, cdLog)
	}
	return nil
}

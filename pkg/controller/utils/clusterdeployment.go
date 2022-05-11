package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func IsDeleteProtected(cd *hivev1.ClusterDeployment) bool {
	protectedDelete, err := strconv.ParseBool(cd.Annotations[constants.ProtectedDeleteAnnotation])
	return protectedDelete && err == nil
}

func IsFakeCluster(cd *hivev1.ClusterDeployment) bool {
	fakeCluster, err := strconv.ParseBool(cd.Annotations[constants.HiveFakeClusterAnnotation])
	return fakeCluster && err == nil
}

// IsClusterPausedOrRelocating checks if the syncing to the cluster is paused or if the cluster is relocating
func IsClusterPausedOrRelocating(cd *hivev1.ClusterDeployment, logger log.FieldLogger) bool {
	if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused {
		logger.WithField("annotation", constants.SyncsetPauseAnnotation).Warn("syncing to cluster is disabled by annotation")
		return true
	}
	if _, relocating := cd.Annotations[constants.RelocateAnnotation]; relocating {
		logger.WithField("annotation", constants.RelocateAnnotation).Info("syncing to cluster is disabled by annotation")
		return true
	}

	return false
}

func IsRelocating(obj metav1.Object) (relocateName string, status hivev1.RelocateStatus, err error) {
	relocateValue, ok := obj.GetAnnotations()[constants.RelocateAnnotation]
	if !ok {
		return
	}
	relocateParts := strings.SplitN(relocateValue, "/", 2)
	if len(relocateParts) != 2 {
		err = errors.New("could not parse")
		return
	}
	relocateName = relocateParts[0]
	status = hivev1.RelocateStatus(relocateParts[1])
	return
}

// SetRelocateAnnotation sets the relocate annotation on the specified object.
func SetRelocateAnnotation(obj metav1.Object, relocateName string, relocateStatus hivev1.RelocateStatus) (changed bool) {
	value := fmt.Sprintf("%s/%s", relocateName, relocateStatus)
	annotations := obj.GetAnnotations()
	changed = annotations[constants.RelocateAnnotation] != value
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[constants.RelocateAnnotation] = value
	obj.SetAnnotations(annotations)
	return
}

func ClearRelocateAnnotation(obj metav1.Object) (changed bool) {
	annotations := obj.GetAnnotations()
	oldLength := len(annotations)
	delete(annotations, constants.RelocateAnnotation)
	if oldLength == len(annotations) {
		return false
	}
	obj.SetAnnotations(annotations)
	return true
}

// CredentialsSecretName returns the name of the credentials secret for platforms
// that have a CredentialsSecretRef. An empty string is returned if platform has none.
func CredentialsSecretName(cd *hivev1.ClusterDeployment) string {
	switch p := cd.Spec.Platform; {
	case p.AWS != nil:
		return cd.Spec.Platform.AWS.CredentialsSecretRef.Name
	case p.GCP != nil:
		return cd.Spec.Platform.GCP.CredentialsSecretRef.Name
	case p.Azure != nil:
		return cd.Spec.Platform.Azure.CredentialsSecretRef.Name
	case p.OpenStack != nil:
		return cd.Spec.Platform.OpenStack.CredentialsSecretRef.Name
	case p.Ovirt != nil:
		return cd.Spec.Platform.Ovirt.CredentialsSecretRef.Name
	case p.BareMetal != nil:
		return ""
	case p.AgentBareMetal != nil:
		return ""
	case p.None != nil:
		return ""
	default:
		return ""
	}
}

// IsClusterDeploymentErrorUpdateEvent returns true when the update event is from
// error state in clusterdeployment.
func IsClusterDeploymentErrorUpdateEvent(evt event.UpdateEvent) bool {
	new, ok := evt.ObjectNew.(*hivev1.ClusterDeployment)
	if !ok {
		return false
	}
	if len(new.Status.Conditions) == 0 {
		return false
	}

	old, ok := evt.ObjectOld.(*hivev1.ClusterDeployment)
	if !ok {
		return false
	}

	for _, cond := range new.Status.Conditions {
		if IsConditionInDesiredState(cond) {
			continue
		}

		oldcond := FindCondition(old.Status.Conditions, cond.Type)
		if oldcond == nil {
			return true // newly added condition in failed state
		}

		if IsConditionInDesiredState(*oldcond) {
			return true // newly Failed condition
		}

		if cond.Message != oldcond.Message ||
			cond.Reason != oldcond.Reason {
			return true // already failing but change in error reported
		}

	}

	return false
}

package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func IsDeleteProtected(cd *hivev1.ClusterDeployment) bool {
	protectedDelete, err := strconv.ParseBool(cd.Annotations[constants.ProtectedDeleteAnnotation])
	return protectedDelete && err == nil
}

func ShouldSyncCluster(cd *hivev1.ClusterDeployment, logger log.FieldLogger) bool {
	if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused {
		logger.WithField("annotation", constants.SyncsetPauseAnnotation).Warn("syncing to cluster is disabled by annotation")
		return false
	}
	if _, relocating := cd.Annotations[constants.RelocateAnnotation]; relocating {
		logger.WithField("annotation", constants.RelocateAnnotation).Info("syncing to cluster is disabled by annotation")
		return false
	}
	return true
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

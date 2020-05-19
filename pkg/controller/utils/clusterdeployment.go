package utils

import (
	"strconv"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

func ShouldSyncCluster(cd *hivev1.ClusterDeployment, logger log.FieldLogger) bool {
	if paused, err := strconv.ParseBool(cd.Annotations[constants.SyncsetPauseAnnotation]); err == nil && paused {
		logger.WithField("annotation", constants.SyncsetPauseAnnotation).Warn("syncing to cluster is disabled by annotation")
		return false
	}
	if _, relocating := cd.Annotations[constants.RelocatingAnnotation]; relocating {
		logger.WithField("annotation", constants.RelocatingAnnotation).Info("syncing to cluster is disabled by annotation")
		return false
	}
	if _, relocated := cd.Annotations[constants.RelocatedAnnotation]; relocated {
		logger.WithField("annotation", constants.RelocatedAnnotation).Info("syncing to cluster is disabled by annotation")
		return false
	}
	return true
}

func IsDeleteProtected(cd *hivev1.ClusterDeployment) bool {
	protectedDelete, err := strconv.ParseBool(cd.Annotations[constants.ProtectedDeleteAnnotation])
	return protectedDelete && err == nil
}

package utils

import (
	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/controller/images"
)

// GetHiveImage looks for a Hive image to use in clusterdeployment jobs in the following order:
// 1 - specified in the cluster deployment spec.images.hiveImage
// 2 - referenced in the cluster deployment spec.imageSet
// 3 - specified via environment variable to the hive controller
// 4 - fallback default hardcoded image reference
func GetHiveImage(cd *hivev1.ClusterDeployment, imageSet *hivev1.ClusterImageSet, cdLog log.FieldLogger) string {
	if cd.Spec.Images.HiveImage != "" {
		return cd.Spec.Images.HiveImage
	}
	if imageSet != nil && imageSet.Spec.HiveImage != nil {
		return *imageSet.Spec.HiveImage
	}
	return images.GetHiveImage(cdLog)
}

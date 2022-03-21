package utils

import (
	"strconv"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// IsClusterMarkedForRemoval returns true when the hive.openshift.io/remove-cluster-from-pool
// annotation is set to true value in the clusterdeployment
func IsClusterMarkedForRemoval(cd *hivev1.ClusterDeployment) bool {
	annotations := []string{
		constants.RemovePoolClusterAnnotation,
		// LEGACY: The annotation used to be exclusively for cascading deletion of a ClusterClaim
		// to deletion of its CD. It has since been renamed, but we need to check for the original
		// annotation in case this code executes on a CD that was marked for deletion by an earlier
		// version.
		// TODO: We should be able to remove this once we're sure no instances of the old
		// annotation exist in the field -- like on a minor version boundary.
		"hive.openshift.io/remove-claimed-cluster-from-pool",
	}
	for _, s := range annotations {
		if v, ok := cd.Annotations[s]; ok && v != "" {
			if b, err := strconv.ParseBool(v); err == nil {
				return b
			}
		}
	}
	return false
}

func MarkClusterForRemoval(cd *hivev1.ClusterDeployment) {
	if cd.Annotations == nil {
		cd.Annotations = map[string]string{}
	}
	cd.Annotations[constants.RemovePoolClusterAnnotation] = "true"
}

package utils

import (
	"strconv"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

// IsClaimedClusterMarkedForRemoval returns true when hive.openshift.io/remove-claimed-cluster-from-pool
// annotation is set to true value in the clusterdeployment
func IsClaimedClusterMarkedForRemoval(cd *hivev1.ClusterDeployment) bool {
	toRemove := false
	if v, ok := cd.Annotations[constants.ClusterClaimRemoveClusterAnnotation]; ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			toRemove = b
		}
	}
	return toRemove
}

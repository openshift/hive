package constants

import (
	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
)

const (
	mergedPullSecretSuffix = "merged-pull-secret"

	// VeleroBackupEnvVar is the name of the environment variable used to tell the controller manager to enable velero backup integration.
	VeleroBackupEnvVar = "HIVE_VELERO_BACKUP"
)

// GetMergedPullSecretName returns name for merged pull secret name per cluster deployment
func GetMergedPullSecretName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, mergedPullSecretSuffix)
}

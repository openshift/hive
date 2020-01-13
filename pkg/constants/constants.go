package constants

import (
	apihelpers "github.com/openshift/hive/pkg/apis/helpers"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
)

const (
	mergedPullSecretSuffix = "merged-pull-secret"

	// VeleroBackupEnvVar is the name of the environment variable used to tell the controller manager to enable velero backup integration.
	VeleroBackupEnvVar = "HIVE_VELERO_BACKUP"

	// MinBackupPeriodSecondsEnvVar is the name of the environment variable used to tell the controller manager the minimum period of time between backups.
	MinBackupPeriodSecondsEnvVar = "HIVE_MIN_BACKUP_PERIOD_SECONDS"

	// SkipGatherLogsEnvVar is the environment variable which passes the configuration to disable
	// log gathering on failed cluster installs. The value will be either "true" or "false".
	// If unset "false" should be assumed. This variable is set by the operator depending on the
	// value of the setting in HiveConfig, passed to the controllers deployment, as well as to
	// install pods which do the actual log gathering.
	SkipGatherLogsEnvVar = "SKIP_GATHER_LOGS"

	// InstallJobLabel is the label used for artifacts specific to Hive cluster installations.
	InstallJobLabel = "hive.openshift.io/install"

	// UninstallJobLabel is the label used for artifacts specific to Hive cluster deprovision.
	UninstallJobLabel = "hive.openshift.io/uninstall"

	// ClusterDeploymentNameLabel is the label that is used to identify the installer pod of a particular cluster deployment
	ClusterDeploymentNameLabel = "hive.openshift.io/cluster-deployment-name"

	// GlobalPullSecret is the environment variable for controllers to get the global pull secret
	GlobalPullSecret = "GLOBAL_PULL_SECRET"

	// HiveNamespace is the name of Hive operator namespace
	HiveNamespace = "hive"

	// CheckpointName is the name of the object in each namespace in which the namespace's backup information is stored.
	CheckpointName = "hive"

	// SyncsetPauseAnnotation is a annotation used by clusterDeployment, if it's true, then we will disable syncing to a specific cluster
	SyncsetPauseAnnotation = "hive.openshift.io/syncset-pause"

	// ManagedDomainsFileEnvVar if present, points to a simple text
	// file that includes a valid managed domain per line. Cluster deployments
	// requesting that their domains be managed must have a base domain
	// that is a direct child of one of the valid domains.
	ManagedDomainsFileEnvVar = "MANAGED_DOMAINS_FILE"

	// GCPCredentialsName is the name of the GCP credentials file or secret key.
	GCPCredentialsName = "osServiceAccount.json"

	// ClusterDeploymentOwnerLabel is the label that is used to identify the owner of a runtime object.
	ClusterDeploymentOwnerLabel = "hive.openshift.io/owner-cluster-deployment-name"

	// ClusterProvisionOwnerLabel is the label that is used to identify the owner of a runtime object.
	ClusterProvisionOwnerLabel = "hive.openshift.io/owner-cluster-provision-name"

	// ClusterDeprovisionOwnerLabel is the label that is used to identify the owner of a runtime object.
	ClusterDeprovisionOwnerLabel = "hive.openshift.io/owner-cluster-deprovision-name"
)

// GetMergedPullSecretName returns name for merged pull secret name per cluster deployment
func GetMergedPullSecretName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, mergedPullSecretSuffix)
}

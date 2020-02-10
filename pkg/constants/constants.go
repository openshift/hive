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

	// ClusterDeploymentNameLabel is the label that is used to identify a relationship to a given cluster deployment object.
	ClusterDeploymentNameLabel = "hive.openshift.io/cluster-deployment-name"

	// ClusterDeprovisionNameLabel is the label that is used to identify a relationship to a given cluster deprovision object.
	ClusterDeprovisionNameLabel = "hive.openshift.io/cluster-deprovision-name"

	// ClusterProvisionNameLabel is the label that is used to identify a relationship to a given cluster provision object.
	ClusterProvisionNameLabel = "hive.openshift.io/cluster-provision-name"

	// SyncSetNameLabel is the label that is used to identify a relationship to a given syncset object.
	SyncSetNameLabel = "hive.openshift.io/syncset-name"

	// SelectorSyncSetNameLabel is the label that is used to identify a relationship to a given selector syncset object.
	SelectorSyncSetNameLabel = "hive.openshift.io/selector-syncset-name"

	// PVCTypeLabel is the label that is used to identify what a PVC is being used for.
	PVCTypeLabel = "hive.openshift.io/pvc-type"

	// PVCTypeInstallLogs is used as a value of PVCTypeLabel that says the PVC specifically stores installer logs.
	PVCTypeInstallLogs = "installlogs"

	// JobTypeLabel is the label that is used to identify what a Job is being used for.
	JobTypeLabel = "hive.openshift.io/job-type"

	// JobTypeImageSet is used as a value of JobTypeLabel that says the Job is specifically running to determine which imageset to use.
	JobTypeImageSet = "imageset"

	// JobTypeDeprovision is used as a value of JobTypeLabel that says the Job is specifically running the deprovisioner.
	JobTypeDeprovision = "deprovision"

	// JobTypeProvision is used as a value of JobTypeLabel that says the Job is specifically running the provisioner.
	JobTypeProvision = "provision"

	// DNSZoneTypeLabel is the label that is used to identify what a DNSZone is being used for.
	DNSZoneTypeLabel = "hive.openshift.io/dnszone-type"

	// DNSZoneTypeChild is used as a value of DNSZoneTypeLabel that says the DNSZone is specifically used as the forwarding zone for the target cluster.
	DNSZoneTypeChild = "child"

	// SecretTypeLabel is the label that is used to identify what a Secret is being used for.
	SecretTypeLabel = "hive.openshift.io/secret-type"

	// SecretTypeMergedPullSecret is used as a value of SecretTypeLabel that says the secret is specifically used for storing a pull secret.
	SecretTypeMergedPullSecret = "merged-pull-secret"

	// SecretTypeKubeConfig is used as a value of SecretTypeLabel that says the secret is specifically used for storing a kubeconfig.
	SecretTypeKubeConfig = "kubeconfig"

	// SecretTypeKubeAdminCreds is used as a value of SecretTypeLabel that says the secret is specifically used for storing kubeadmin credentials.
	SecretTypeKubeAdminCreds = "kubeadmincreds"

	// SyncSetTypeLabel is the label that is used to identify what a SyncSet is being used for.
	SyncSetTypeLabel = "hive.openshift.io/syncset-type"

	// SyncSetTypeControlPlaneCerts is used as a value of SyncSetTypeLabel that says the syncset is specifically used to distribute control plane certificates.
	SyncSetTypeControlPlaneCerts = "controlplanecerts"

	// SyncSetTypeRemoteIngress is used as a value of SyncSetTypeLabel that says the syncset is specifically used to distribute remote ingress information.
	SyncSetTypeRemoteIngress = "remoteingress"

	// SyncSetTypeIdentityProvider is used as a value of SyncSetTypeLabel that says the syncset is specifically used to distribute identity provider information.
	SyncSetTypeIdentityProvider = "identityprovider"

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
)

// GetMergedPullSecretName returns name for merged pull secret name per cluster deployment
func GetMergedPullSecretName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, mergedPullSecretSuffix)
}

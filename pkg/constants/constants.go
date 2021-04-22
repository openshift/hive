package constants

import (
	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
)

const (
	PlatformAWS            = "aws"
	PlatformAzure          = "azure"
	PlatformBaremetal      = "baremetal"
	PlatformAgentBaremetal = "agent-baremetal"
	PlatformGCP            = "gcp"
	PlatformOpenStack      = "openstack"
	PlatformUnknown        = "unknown"
	PlatformVSphere        = "vsphere"

	mergedPullSecretSuffix = "merged-pull-secret"

	// VeleroBackupEnvVar is the name of the environment variable used to tell the controller manager to enable velero backup integration.
	VeleroBackupEnvVar = "HIVE_VELERO_BACKUP"

	// VeleroNamespaceEnvVar is the name of the environment variable used to tell the controller manager which namespace velero backup objects should be created in.
	VeleroNamespaceEnvVar = "HIVE_VELERO_NAMESPACE"

	// DeprovisionsDisabledEnvVar is the name of the environment variable used to tell the controller manager to skip
	// processing of any ClusterDeprovisions.
	DeprovisionsDisabledEnvVar = "DEPROVISIONS_DISABLED"

	// MinBackupPeriodSecondsEnvVar is the name of the environment variable used to tell the controller manager the minimum period of time between backups.
	MinBackupPeriodSecondsEnvVar = "HIVE_MIN_BACKUP_PERIOD_SECONDS"

	// InstallJobLabel is the label used for artifacts specific to Hive cluster installations.
	InstallJobLabel = "hive.openshift.io/install"

	// UninstallJobLabel is the label used for artifacts specific to Hive cluster deprovision.
	UninstallJobLabel = "hive.openshift.io/uninstall"

	// MachinePoolNameLabel is the label that is used to identify the MachinePool which owns a particular resource.
	MachinePoolNameLabel = "hive.openshift.io/machine-pool-name"

	// ClusterDeploymentNameLabel is the label that is used to identify a relationship to a given cluster deployment object.
	ClusterDeploymentNameLabel = "hive.openshift.io/cluster-deployment-name"

	// ClusterDeprovisionNameLabel is the label that is used to identify a relationship to a given cluster deprovision object.
	ClusterDeprovisionNameLabel = "hive.openshift.io/cluster-deprovision-name"

	// ClusterProvisionNameLabel is the label that is used to identify a relationship to a given cluster provision object.
	ClusterProvisionNameLabel = "hive.openshift.io/cluster-provision-name"

	// ClusterPoolNameLabel is the label that is used to signal that a namespace was created to house a
	// ClusterDeployment created for a ClusterPool. The label is used to reap namespaces after the ClusterDeployment
	// has been deleted.
	ClusterPoolNameLabel = "hive.openshift.io/cluster-pool-name"

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

	// DefaultHiveNamespace is the default namespace where core hive components will run. It is used if the environment variable is not defined.
	DefaultHiveNamespace = "hive"

	// HiveNamespaceEnvVar is the environment variable for the namespace where the core hive-controllers and hiveadmission will run.
	// This is set on the deployments by the hive-operator which deploys them, based on the targetNamespace defined in HiveConfig.
	// The default is defined above.
	HiveNamespaceEnvVar = "HIVE_NS"

	// CheckpointName is the name of the object in each namespace in which the namespace's backup information is stored.
	CheckpointName = "hive"

	// SyncsetPauseAnnotation is a annotation used by clusterDeployment, if it's true, then we will disable syncing to a specific cluster
	SyncsetPauseAnnotation = "hive.openshift.io/syncset-pause"

	// HiveManagedLabel is a label added to any resources we sync to the remote cluster to help identify that they are
	// managed by Hive, and any manual changes may be undone the next time the resource is reconciled.
	HiveManagedLabel = "hive.openshift.io/managed"

	// DisableInstallLogPasswordRedactionAnnotation is an annotation used on ClusterDeployments to disable the installmanager
	// functionality which refuses to print output if it appears to contain a password or sensitive info. This can be
	// useful in scenarios where debugging is needed and important info is being redacted. Set to "true".
	DisableInstallLogPasswordRedactionAnnotation = "hive.openshift.io/disable-install-log-password-redaction"

	// PauseOnInstallFailureAnnotation is an annotation used on ClusterDeployments to trigger a sleep after an install
	// failure for the specified duration. This will keep the install pod running and allow a user to rsh in for debug
	// purposes. Examples: "1h", "20m".
	PauseOnInstallFailureAnnotation = "hive.openshift.io/pause-on-install-failure"

	// WaitForInstallCompleteExecutionsAnnotation is an annotation used on ClusterDeployments to set additional waits
	// for the cluster provision to complete by running `openshift-install wait-for install-complete` command.
	WaitForInstallCompleteExecutionsAnnotation = "hive.openshift.io/wait-for-install-complete-executions"

	// ProtectedDeleteAnnotation is an annotation used on ClusterDeployments to indicate that the ClusterDeployment
	// cannot be deleted. The annotation must be removed in order to delete the ClusterDeployment.
	ProtectedDeleteAnnotation = "hive.openshift.io/protected-delete"

	// ProtectedDeleteEnvVar is the name of the environment variable used to tell the controller manager whether
	// protected delete is enabled.
	ProtectedDeleteEnvVar = "PROTECTED_DELETE"

	// RelocateAnnotation is an annotation used on ClusterDeployments and DNSZones to indicate that the resource
	// is involved in a relocation between Hive instances.
	// The value of the annotation has the format "{ClusterRelocate}/{Status}", where
	// {ClusterRelocate} is the name of the ClusterRelocate that is driving the relocation and
	// {Status} is the status of the relocate. The status is outgoing, completed, or incoming.
	// An outgoing status indicates that the resource is on the source side of an in-progress relocate.
	// A completed status indicates that the resource is on the source side of a completed relocate.
	// An incoming status indicates that the resource is on the destination side of an in-progress relocate.
	RelocateAnnotation = "hive.openshift.io/relocate"

	// ManagedDomainsFileEnvVar if present, points to a simple text
	// file that includes a valid managed domain per line. Cluster deployments
	// requesting that their domains be managed must have a base domain
	// that is a direct child of one of the valid domains.
	ManagedDomainsFileEnvVar = "MANAGED_DOMAINS_FILE"

	// SupportedContractImplementationsFileEnvVar if present, points to a simple json
	// file that includes a list of contracts and their supported implementations.
	SupportedContractImplementationsFileEnvVar = "SUPPORTED_CONTRACT_IMPLEMENTATIONS_FILE"

	// ManagedDomainsVolumeName is the name of the volume that will point
	// to the configmap containing the managed domain configuration.
	ManagedDomainsVolumeName = "managed-domains"

	// GCPCredentialsName is the name of the GCP credentials file or secret key.
	GCPCredentialsName = "osServiceAccount.json"

	// AzureCredentialsName is the name of the Azure credentials file or secret key.
	AzureCredentialsName = "osServicePrincipal.json"

	// AzureCredentialsEnvVar is the name of the environment variable pointing to the location
	// where Azure credentials can be found.
	AzureCredentialsEnvVar = "AZURE_AUTH_LOCATION"

	// OpenStackCredentialsName is the name of the OpenStack credentials file.
	OpenStackCredentialsName = "clouds.yaml"

	// SSHPrivKeyPathEnvVar is the environment variable Hive will set for the installmanager pod to point to the
	// path where we mount in the SSH key to be configured on the cluster hosts.
	SSHPrivKeyPathEnvVar = "SSH_PRIV_KEY_PATH"

	// LibvirtSSHPrivKeyPathEnvVar is the environment variable Hive will set for the installmanager pod to point to the
	// path where we mount in the SSH key for connecting to the bare metal libvirt provisioning host.
	LibvirtSSHPrivKeyPathEnvVar = "LIBVIRT_SSH_PRIV_KEY_PATH"

	// BoundServiceAccountSigningKeyEnvVar contains the path to the bound service account signing key and
	// is set in the install pod for AWS STS clusters.
	BoundServiceAccountSigningKeyEnvVar = "BOUND_SA_SIGNING_KEY"

	// BoundServiceAccountSigningKeyFile is the Secret key and filename where a
	// ServiceAccount signing key will be projected into the install pod.
	BoundServiceAccountSigningKeyFile = "bound-service-account-signing-key.key"

	// FakeClusterInstallEnvVar is the environment variable Hive will set for the installmanager pod to request
	// a fake install.
	FakeClusterInstallEnvVar = "FAKE_INSTALL"

	// ControlPlaneCertificateSuffix is the suffix used when naming objects having to do control plane certificates.
	ControlPlaneCertificateSuffix = "cp-certs"

	// ClusterIngressSuffix is the suffix used when naming objects having to do with cluster ingress.
	ClusterIngressSuffix = "clusteringress"

	// IdentityProviderSuffix is the suffix used when naming objects having to do with identity provider
	IdentityProviderSuffix = "idp"

	// KubeconfigSecretKey is the key used inside of a secret containing a kubeconfig
	KubeconfigSecretKey = "kubeconfig"

	// UsernameSecretKey is a key used to store a username inside of a secret containing username / password credentials
	UsernameSecretKey = "username"

	// PasswordSecretKey is a key used to store a password inside of a secret containing username / password credentials
	PasswordSecretKey = "password"

	// AWSRoute53Region is the region to use for route53 operations.
	AWSRoute53Region = "us-east-1"

	// AWSChinaRoute53Region is the region to use for AWS China route53 operations.
	AWSChinaRoute53Region = "cn-northwest-1"

	// AWSChinaRegionPrefix is the prefix for regions in AWS China.
	AWSChinaRegionPrefix = "cn-"

	// SSHPrivateKeySecretKey is the key we use in a Kubernetes Secret containing an SSH private key.
	SSHPrivateKeySecretKey = "ssh-privatekey"

	// RawKubeconfigSecretKey is the key we use in a Kubernetes Secret containing the raw (unmodified) form of
	// an admin kubeconfig. (before Hive injects things such as additional CAs)
	RawKubeconfigSecretKey = "raw-kubeconfig"

	// AWSAccessKeyIDSecretKey is the key we use in a Kubernetes Secret containing AWS credentials for the access key ID.
	AWSAccessKeyIDSecretKey = "aws_access_key_id"

	// AWSSecretAccessKeySecretKey is the key we use in a Kubernetes Secret containing AWS credentials for the access key ID.
	AWSSecretAccessKeySecretKey = "aws_secret_access_key"

	// AWSConfigSecretKey is the key we use in a Kubernetes Secret containing AWS config.
	AWSConfigSecretKey = "aws_config"

	// AWSCredsMount is the location where the AWS credentials secret is mounted for uninstall pods.
	AWSCredsMount = "/etc/aws-creds"

	// TLSCrtSecretKey is the key we use in a Kubernetes Secret containing a TLS certificate.
	TLSCrtSecretKey = "tls.crt"

	// TLSKeySecretKey is the key we use in a Kubernetes Secret containing a TLS certificate key.
	TLSKeySecretKey = "tls.key"

	// VSphereUsernameEnvVar is the environent variable specifying the vSphere username.
	VSphereUsernameEnvVar = "GOVC_USERNAME"

	// VSpherePasswordEnvVar is the environment variable specifying the vSphere password.
	VSpherePasswordEnvVar = "GOVC_PASSWORD"

	// VSphereVCenterEnvVar is the environment variable specifying the vSphere vCenter host.
	VSphereVCenterEnvVar = "GOVC_HOST"

	// VSphereTLSCACertsEnvVar is the environment variable containing : delimited paths to vSphere CA certificates.
	VSphereTLSCACertsEnvVar = "GOVC_TLS_CA_CERTS"

	// VSphereNetworkEnvVar is the environment variable specifying the vSphere network.
	VSphereNetworkEnvVar = "GOVC_NETWORK"

	// VSphereDataCenterEnvVar is the environment variable specifying the vSphere datacenter.
	VSphereDataCenterEnvVar = "GOVC_DATACENTER"

	// VSphereDataStoreEnvVar is the environment variable specifying the vSphere default datastore.
	VSphereDataStoreEnvVar = "GOVC_DATASTORE"

	// VersionMajorLabel is a label applied to ClusterDeployments to show the version of the cluster
	// in the form "[MAJOR]".
	VersionMajorLabel = "hive.openshift.io/version-major"

	// VersionMajorMinorLabel is a label applied to ClusterDeployments to show the version of the cluster
	// in the form "[MAJOR].[MINOR]".
	VersionMajorMinorLabel = "hive.openshift.io/version-major-minor"

	// VersionMajorMinorPatchLabel is a label applied to ClusterDeployments to show the version of the cluster
	// in the form "[MAJOR].[MINOR].[PATCH]".
	VersionMajorMinorPatchLabel = "hive.openshift.io/version-major-minor-patch"

	// OvirtCredentialsName is the name of the oVirt credentials file.
	OvirtCredentialsName = "ovirt-config.yaml"

	// OvirtConfigEnvVar is the environment variable specifying the oVirt config path
	OvirtConfigEnvVar = "OVIRT_CONFIG"

	// InstallLogsUploadProviderEnvVar is used to specify which object store provider is being used.
	InstallLogsUploadProviderEnvVar = "HIVE_INSTALL_LOGS_UPLOAD_PROVIDER"

	// InstallLogsUploadProviderAWS is used to specify that AWS is the cloud provider to upload logs to.
	InstallLogsUploadProviderAWS = "aws"

	// InstallLogsCredentialsSecretRefEnvVar is the environment variable specifying what secret to use for storing logs.
	InstallLogsCredentialsSecretRefEnvVar = "HIVE_INSTALL_LOGS_CREDENTIALS_SECRET"

	// InstallLogsAWSRegionEnvVar is the environment variable specifying the region to use with S3
	InstallLogsAWSRegionEnvVar = "HIVE_INSTALL_LOGS_AWS_REGION"

	// InstallLogsAWSServiceEndpointEnvVar is the environment variable specifying the S3 endpoint to use.
	InstallLogsAWSServiceEndpointEnvVar = "HIVE_INSTALL_LOGS_AWS_S3_URL"

	// InstallLogsAWSS3BucketEnvVar is the environment variable specifying the S3 bucket to use.
	InstallLogsAWSS3BucketEnvVar = "HIVE_INSTALL_LOGS_AWS_S3_BUCKET"

	// HiveFakeClusterAnnotation can be set to true on a cluster deployment to create a fake cluster that never
	// provisions resources, and all communication with the cluster will be faked.
	HiveFakeClusterAnnotation = "hive.openshift.io/fake-cluster"

	// ReconcileIDLen is the length of the random strings we generate for contextual loggers in controller
	// Reconcile functions.
	ReconcileIDLen = 8

	// SyncSetMetricsGroupAnnotation can be applied to non-selector SyncSets to make them part of a
	// group for which first applied metrics can be reported
	SyncSetMetricsGroupAnnotation = "hive.openshift.io/syncset-metrics-group"

	// ClusterClaimRemoveClusterAnnotation is used by the cluster claim controller to mark that the cluster
	// that are previously claimed is no longer required and therefore should be removed/deprovisioned and removed
	// from the pool.
	ClusterClaimRemoveClusterAnnotation = "hive.openshift.io/remove-claimed-cluster-from-pool"

	// HiveAWSServiceProviderCredentialsSecretRefEnvVar is the environment variable specifying what secret to use for
	// assuming the service provider credentials for AWS clusters.
	HiveAWSServiceProviderCredentialsSecretRefEnvVar = "HIVE_AWS_SERVICE_PROVIDER_CREDENTIALS_SECRET"

	// HiveFeatureGatesEnabledEnvVar is the the environment variable specifying the comma separated list of
	// feature gates that are enabled.
	HiveFeatureGatesEnabledEnvVar = "HIVE_FEATURE_GATES_ENABLED"

	// MachineManagementAnnotation
	MachineManagementAnnotation = "hive.openshift.io/machine-management-cluster-name"

	// AWSPrivateLinkControllerConfigFileEnvVar if present, points to a simple text
	// file that includes configuration for aws-private-link-controller
	AWSPrivateLinkControllerConfigFileEnvVar = "AWS_PRIVATELINK_CONTROLLER_CONFIG_FILE"
)

// GetMergedPullSecretName returns name for merged pull secret name per cluster deployment
func GetMergedPullSecretName(cd *hivev1.ClusterDeployment) string {
	return apihelpers.GetResourceName(cd.Name, mergedPullSecretSuffix)
}

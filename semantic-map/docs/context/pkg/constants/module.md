# Module atlas

## Responsibility

Defines all shared constant values used across the Hive codebase -- platform identifiers, label/annotation keys, environment variable names, secret keys, directory paths, and time durations. Tests exist.

## Public Interface/API

**Constants (170+ exported, grouped by category):**

*Platform identifiers:*
- `PlatformAWS`, `PlatformAzure`, `PlatformGCP`, `PlatformIBMCloud`, `PlatformOpenStack`, `PlatformVSphere`, `PlatformNutanix`, `PlatformBaremetal`, `PlatformAgentBaremetal`, `PlatformNone`, `PlatformUnknown`

*Labels:*
- `ClusterDeploymentNameLabel`, `ClusterDeprovisionNameLabel`, `ClusterProvisionNameLabel`, `ClusterPoolNameLabel`, `HiveManagedLabel`, `CreatedByHiveLabel`, `InstallJobLabel`, `UninstallJobLabel`, `JobTypeLabel`, `DNSZoneTypeLabel`, `VersionLabel`, `VersionMajorLabel`, `VersionMajorMinorLabel`, `VersionMajorMinorPatchLabel`, etc.

*Annotations:*
- `SyncsetPauseAnnotation`, `PowerStatePauseAnnotation`, `ReconcilePauseAnnotation`, `InfraDisabledAnnotation`, `DisableInstallLogPasswordRedactionAnnotation`, `ProtectedDeleteAnnotation`, `RelocateAnnotation`, `AdditionalLogFieldsAnnotation`, `ExtraWorkerSecurityGroupAnnotation`, `ClusterDeploymentPoolSpecHashAnnotation`, `HiveFakeClusterAnnotation`, `OverrideMachinePoolPlatformAnnotation`, `MinimalInstallModeAnnotation`, `CopyCLIImageDomainFromInstallerImage`, `LegacyDeprovisionAnnotation`, etc.

*Environment variables:*
- `HiveNamespaceEnvVar`, `FakeClusterInstallEnvVar`, `DeprovisionsDisabledEnvVar`, `HiveFeatureGatesEnabledEnvVar`, `AWSPrivateLinkControllerConfigFileEnvVar`, `FailedProvisionConfigFileEnvVar`, `IBMCloudAPIKeyEnvVar`, `VSphereUsernameEnvVar`, `VSpherePasswordEnvVar`, `AdditionalLogFieldsEnvVar`, `InstallLogsUploadProviderEnvVar`, `ClusterVersionPollIntervalEnvVar`, etc.

*Secret/config keys:*
- `KubeconfigSecretKey`, `RawKubeconfigSecretKey`, `MetadataJSONSecretKey`, `AWSAccessKeyIDSecretKey`, `AWSSecretAccessKeySecretKey`, `AWSConfigSecretKey`, `SSHPrivateKeySecretKey`, `UsernameSecretKey`, `PasswordSecretKey`, `VSphereVCentersSecretKey`, `IBMCloudAPIKeySecretKey`, `TLSCrtSecretKey`, `TLSKeySecretKey`, `GCPCredentialsName`, `AzureCredentialsName`, `BoundServiceAccountSigningKeyFile`, etc.

*Directories and paths:*
- `GCPCredentialsDir`, `AzureCredentialsDir`, `SSHPrivateKeyDir`, `VSphereCredentialsDir`, `VSphereCertificatesDir`, `OpenStackCredentialsDir`, `OpenStackCertificatesDir`, `BoundServiceAccountSigningKeyDir`, `TrustedCABundleDir`, `NutanixCertificatesDir`

*Nutanix CLI option names (in nutanix.go):*
- `CliNutanixPcAddressOpt`, `CliNutanixPcPortOpt`, `CliNutanixPeAddressOpt`, `CliNutanixPePortOpt`, `CliNutanixPeUUIDOpt`, `CliNutanixPeNameOpt`, `CliNutanixAzNameOpt`, `CliNutanixApiVipOpt`, `CliNutanixIngressVipOpt`, `CliNutanixSubnetUUIDOpt`, `CliNutanixCACertsOpt`, `NutanixUsernameEnvVar`, `NutanixPasswordEnvVar`

*Other:*
- `DefaultHiveNamespace`, `HiveConfigName`, `CheckpointName`, `ClusterOperatorSettlePause` (2m duration), `ReconcileIDLen`, `AWSRoute53Region`, `AWSChinaRoute53Region`, `AWSChinaRegionPrefix`

**Functions:**
- `GetMergedPullSecretName(cd *hivev1.ClusterDeployment) string` -- Returns the merged pull secret name for a ClusterDeployment

## Internal Dependencies

- `github.com/openshift/hive/apis/helpers` -- `GetResourceName` for name generation
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment type
- `time` -- Duration constants

## Capabilities

- Central definition of all platform identifiers for cloud provider switching
- Standard label and annotation keys used across all Hive controllers and webhooks
- Environment variable names for controller configuration plumbing (from HiveConfig through hive-operator to controllers)
- Secret and credential key names for consistent secret data access
- Directory paths for credential and certificate mounts
- Nutanix-specific CLI option names for the hiveutil create-cluster command

## Understanding Score

0.9

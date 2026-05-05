# Module atlas

## Responsibility

Central constants package defining labels, annotations, environment variables, secret keys, directory paths, and platform identifiers used across all Hive controllers, operators, and CLI tools.

## Public Interface/API

- Platform identifiers: `PlatformAWS`, `PlatformAzure`, `PlatformGCP`, `PlatformIBMCloud`, `PlatformOpenStack`, `PlatformVSphere`, `PlatformNutanix`, `PlatformBaremetal`, `PlatformNone`, `PlatformUnknown`, `PlatformAgentBaremetal`
- Labels: `ClusterDeploymentNameLabel`, `ClusterPoolNameLabel`, `InstallJobLabel`, `UninstallJobLabel`, `MachinePoolNameLabel`, `HiveManagedLabel`, `VersionLabel`, `VersionMajorMinorPatchLabel`, etc.
- Annotations: `ReconcilePauseAnnotation`, `SyncsetPauseAnnotation`, `PowerStatePauseAnnotation`, `ProtectedDeleteAnnotation`, `RelocateAnnotation`, `HiveFakeClusterAnnotation`, `PauseOnInstallFailureAnnotation`, etc.
- Environment variables: `HiveNamespaceEnvVar`, `GlobalPullSecret`, `VeleroBackupEnvVar`, `DeprovisionsDisabledEnvVar`, `HiveFeatureGatesEnabledEnvVar`, `FailedProvisionConfigFileEnvVar`, etc.
- Secret keys: `KubeconfigSecretKey`, `AWSAccessKeyIDSecretKey`, `AWSSecretAccessKeySecretKey`, `AWSConfigSecretKey`, `GCPCredentialsName`, `AzureCredentialsName`, `IBMCloudAPIKeySecretKey`, etc.
- Credential/certificate directory paths: `AWSCredsMount`, `GCPCredentialsDir`, `AzureCredentialsDir`, `VSphereCredentialsDir`, `SSHPrivateKeyDir`, etc.
- `GetMergedPullSecretName(cd) string` — derives merged pull secret name from ClusterDeployment
- `ClusterOperatorSettlePause` — 2-minute duration for CO readiness check delay

## Internal Dependencies

- `github.com/openshift/hive/apis/helpers` — `GetResourceName` for name derivation
- `github.com/openshift/hive/apis/hive/v1` — ClusterDeployment type for `GetMergedPullSecretName`

## Capabilities

- Single source of truth for ~170+ exported constants used across the entire Hive codebase
- Defines the labeling and annotation taxonomy for Hive resources
- Platform identifiers used in credential loading, controller routing, and install-config generation
- Environment variable names used to configure controller behavior via HiveConfig operator

## Understanding Score

0.85

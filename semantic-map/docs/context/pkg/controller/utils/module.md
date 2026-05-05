# Module atlas

## Responsibility

Large shared utility package used by virtually all Hive controllers. Provides common functionality including: condition management (initialize/set/find conditions for ClusterDeployment, ClusterClaim, ClusterPool, MachinePool), finalizer management, expectations tracking (TTL-based creation/deletion tracking), controller configuration (rate limiting, concurrency from env vars and HiveConfig), client wrappers with metrics, kubeconfig handling, secret loading, job/StatefulSet hash calculation, error scrubbing for CRD status, RBAC helpers, credential validation, DNS zone utilities, log field management, rate-limited event handlers, delaying reconciler wrapper, watcher injection, and various platform-specific helpers.

## Public Interface/API

Key exports (137+ total, showing most important):

- **Conditions**: `InitializeClusterDeploymentConditions`, `SetClusterDeploymentCondition`, `SetClusterDeploymentConditionWithChangeCheck`, `FindCondition`, `IsConditionInDesiredState`, `AreAllConditionsInDesiredState`, plus equivalents for ClusterClaim, ClusterPool, MachinePool.
- **Finalizers**: `HasFinalizer`, `AddFinalizer`, `DeleteFinalizer`.
- **Expectations**: `ExpectationsInterface`, `Expectations`, `NewExpectations`, `ControlleeExpectations`.
- **Controller config**: `GetControllerConfig`, `NewClientWithMetricsOrDie`, `AddControllerMetricsTransportWrapper`.
- **Client/kubeconfig**: `LoadSecretData`, `RestConfigFromSecret`, `CopySecret`, `AddAdditionalKubeconfigCAs`.
- **Job utilities**: `IsFinished`, `IsSuccessful`, `IsFailed`, `IsDeadlineExceeded`, `CalculateJobSpecHash`.
- **Cluster state**: `IsClusterPausedOrRelocating`, `IsRelocating`, `IsClusterMarkedForRemoval`, `MarkClusterForRemoval`, `IsFakeCluster`, `IsDeleteProtected`, `InfraDisabled`.
- **DNS**: `DNSZoneName`, `Dotted`.
- **Logging**: `BuildControllerLogger`, `AddLogFields`, `ExtractLogFields`, `LogLevel`.
- **Error scrubbing**: `ErrorScrub`.
- **Owner references**: `ReconcileOwnerReferences`.
- **Platform helpers**: `GetClusterPlatform`, `CredentialsSecretName`, `ValidateCredentialsForClusterDeployment`.
- **RBAC**: `InstallServiceAccountName`.
- **Event handling**: `NewTypedRateLimitedUpdateEventHandler`, `IsClusterDeploymentErrorUpdateEvent`.
- **Reconciler wrappers**: `NewDelayingReconciler`, `InjectWatcher`, `Watcher`.
- **Pod config**: `ReadSharedConfigFromThisPod`, `SharedPodConfig`.
- **StatefulSet**: `GetMyOrdinalID`, `CalculateStatefulSetSpecHash`.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- All core CRD types.
- `github.com/openshift/hive/apis/helpers` -- API helper utilities.
- `github.com/openshift/hive/pkg/constants` -- All constant definitions.
- `github.com/openshift/installer/pkg/types/vsphere` -- vSphere credential validation.

## Capabilities

Pure utility package -- no controller or reconciler. Used as the shared foundation for all Hive controllers. Contains ~35 Go source files covering conditions, client wrappers, expectations, credentials, DNS, errors, jobs, logging, ownership, pod config, RBAC, rate limiting, secrets, StatefulSets, taints, and general utilities.

## Understanding Score

0.75

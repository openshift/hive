# Module atlas

## Responsibility

Shared utility library for all Hive controllers. Provides condition management for every Hive CRD type, controller configuration (concurrency, rate limiting), metrics-instrumented Kubernetes clients, finalizer helpers, owner reference reconciliation, expectations (create/delete tracking), job/statefulset utilities, credential validation, DNS zone helpers, kubeconfig/secret loading, service account setup, error scrubbing, proxy env propagation, and other cross-cutting controller concerns.

## Public Interface/API

**Controller setup and configuration:**
- `func GetControllerConfig(client client.Client, controllerName hivev1.ControllerName) (int, flowcontrol.RateLimiter, workqueue.TypedRateLimiter[reconcile.Request], error)`
- `func NewClientWithMetricsOrDie(mgr manager.Manager, ctrlrName hivev1.ControllerName, rateLimiter *flowcontrol.RateLimiter) client.Client`
- `func AddControllerMetricsTransportWrapper(cfg *rest.Config, controllerName hivev1.ControllerName, remote bool)`
- `func NewDelayingReconciler(r reconcile.Reconciler, logger log.FieldLogger) reconcile.Reconciler`
- `func BuildControllerLogger(controller hivev1.ControllerName, resource string, nsName types.NamespacedName) *log.Entry`
- `const ConcurrentReconcilesEnvVariableFormat, ClientQPSEnvVariableFormat, ClientBurstEnvVariableFormat, QueueQPSEnvVariableFormat, QueueBurstEnvVariableFormat`

**Condition management (per CRD type):**
- `func InitializeClusterDeploymentConditions(existingConditions, conditionsToBeAdded) ([]hivev1.ClusterDeploymentCondition, bool)`
- `func SetClusterDeploymentCondition(...) []hivev1.ClusterDeploymentCondition`
- `func SetClusterDeploymentConditionWithChangeCheck(...) ([]hivev1.ClusterDeploymentCondition, bool)`
- `func SortClusterDeploymentConditions(conditions) []hivev1.ClusterDeploymentCondition`
- `func InitializeClusterClaimConditions(...)`, `func SetClusterClaimCondition(...)`, `func SetClusterClaimConditionWithChangeCheck(...)`
- `func InitializeClusterPoolConditions(...)`, `func SetClusterPoolCondition(...)`, `func SetClusterPoolConditionWithChangeCheck(...)`
- `func SetClusterProvisionCondition(...)`
- `func SetSyncCondition(...)`
- `func SetDNSZoneCondition(...)`, `func SetDNSZoneConditionWithChangeCheck(...)`
- `func InitializeMachinePoolConditions(...)`, `func SetMachinePoolCondition(...)`, `func SetMachinePoolConditionWithChangeCheck(...)`
- `func SetClusterDeprovisionCondition(...)`, `func SetClusterDeprovisionConditionWithChangeCheck(...)`
- `func SetClusterInstallConditionWithChangeCheck(...)`
- `func FindCondition[C, T](conditions []C, conditionType T) *C`
- `func IsConditionInDesiredState(condition) bool`
- `func IsConditionWithPositivePolarity(conditionType) bool`
- `func AreAllConditionsInDesiredState(conditions) bool`
- `type UpdateConditionCheck func(oldReason, oldMessage, newReason, newMessage string) bool`
- `func UpdateConditionAlways(...)`, `func UpdateConditionNever(...)`, `func UpdateConditionIfReasonOrMessageChange(...)`

**Finalizer helpers:**
- `func HasFinalizer(object metav1.Object, finalizer string) bool`
- `func AddFinalizer(object metav1.Object, finalizer string)`
- `func DeleteFinalizer(object metav1.Object, finalizer string)`

**ClusterDeployment inspection:**
- `func IsDeleteProtected(cd) bool`
- `func IsFakeCluster(cd) bool`
- `func IsClusterPausedOrRelocating(cd, logger) bool`
- `func IsRelocating(obj metav1.Object) (string, hivev1.RelocateStatus, error)`
- `func SetRelocateAnnotation(obj, relocateName, relocateStatus) bool`
- `func ClearRelocateAnnotation(obj metav1.Object) bool`
- `func InfraDisabled(cd, controllerName) bool`
- `func CredentialsSecretName(cd) string`
- `func IsClusterDeploymentErrorUpdateEvent(evt event.UpdateEvent) bool`
- `func IsClusterMarkedForRemoval(cd) bool`
- `func MarkClusterForRemoval(cd)`
- `func GetClusterPlatform(cd) string`
- `func AWSHostedZoneRole(cd) *string`
- `func AzureResourceGroup(cd) (string, error)`
- `func GCPNetworkProjectID(cd) *string`
- `func LoadManifestPatches(c, cd, log) ([]hivev1.InstallerManifestPatch, error)`

**Ownership and references:**
- `type OwnershipUniqueKey struct`
- `func ReconcileOwnerReferences(owner, ownershipKeys, kubeclient, scheme, logger) error`
- `func SyncOwnerReference(owner, object, kubeclient, scheme, controlled, logger) error`

**Expectations (create/delete tracking):**
- `type ExpectationsInterface interface`
- `type Expectations struct`
- `type ControlleeExpectations struct`
- `func NewExpectations(logger) *Expectations`
- `var ExpKeyFunc`
- `const ExpectationsTimeout`

**Job/StatefulSet utilities:**
- `func IsSuccessful(job) bool`, `func IsFailed(job) bool`, `func IsFinished(job) bool`, `func IsDeadlineExceeded(job) bool`
- `func CalculateJobSpecHash(job) (string, error)`
- `func CalculateStatefulSetSpecHash(statefulset) (string, error)`
- `func GetMyOrdinalID(controllerName, logger) (int64, error)`
- `func IsUIDAssignedToMe(c, deploymentName, uid, myOrdinalID, logger) (bool, error)`

**Secrets and kubeconfig:**
- `func LoadSecretData(c, secretName, namespace, dataKey) (string, error)`
- `func RestConfigFromSecret(kubeconfigSecret, tryRaw) (*rest.Config, error)`
- `func CopySecret(c, src, dest, owner, scheme) (bool, error)`
- `func AWSServiceProviderSecretName(prefix string) string`

**CA and credentials:**
- `func SetupAdditionalCA() error`
- `func AddAdditionalKubeconfigCAs(data []byte) ([]byte, error)`
- `func TrustBundleFromSecretToWriter(c, secretNamespace, secretName, w) error`
- `func ValidateCredentialsForClusterDeployment(kubeClient, cd, logger) (bool, error)`

**DNS:**
- `func DNSZoneName(cdName string) string`
- `func EnqueueDNSZonesOwnedByClusterDeployment(c, logger) handler.TypedEventHandler`
- `func ReconcileDNSZoneForRelocation(c, logger, dnsZone, finalizer) (*reconcile.Result, error)`

**Domain helpers:**
- `func Dotted(domain string) string`
- `func Undotted(domain string) string`

**Metrics transport:**
- `type ControllerMetricsTripper struct`

**Duck typing:**
- `func GetDuckType(ctx, client, gvk, key, obj) error`

**Error handling:**
- `func ErrorScrub(err error) string`
- `func LogLevel(err error) log.Level`

**Logging and annotations:**
- `type AdditionalLogFieldHavinThing interface`
- `type MetaObjectLogTagger struct`
- `type StringLogTagger struct`
- `func ExtractLogFields[O](obj O) (map[string]any, error)`
- `func AddLogFields[O](obj O, logger) *log.Entry`
- `func AddLogFieldsEnvVar(from metav1.Object, to *batchv1.Job)`
- `func CopyAnnotations(from, to metav1.Object, keys ...string) bool`
- `func CopyLogAnnotation(from, to metav1.Object) bool`

**Event handlers:**
- `func NewRateLimitedUpdateEventHandler(eventHandler, shouldRateLimitFunc) handler.EventHandler`
- `func NewTypedRateLimitedUpdateEventHandler[T, C](typedEventHandler, shouldRateLimitFunc) handler.TypedEventHandler[T, C]`

**Service accounts:**
- `func SetupClusterInstallServiceAccount(c, namespace, logger) error`
- `func SetupClusterUninstallServiceAccount(c, namespace, logger) error`
- `const InstallServiceAccountName, UninstallServiceAccountName`

**Taints:**
- `func GetUniqueTaints(taints) *[]corev1.Taint`
- `func IdentifierForTaint(taint) hivev1.TaintIdentifier`
- `func ListFromTaintMap(taintMap) *[]corev1.Taint`
- `func TaintExists(taintMap, taint) bool`

**Pod config:**
- `type SharedPodConfig struct`
- `func ReadSharedConfigFromThisPod(cl client.Client) (*SharedPodConfig, error)`

**Watcher injection:**
- `type Watcher interface`
- `func InjectWatcher(r reconcile.Reconciler, c controller.Controller)`

**Misc:**
- `func GetChecksumOfObject(object any) (string, error)`
- `func GetChecksumOfObjects(objects ...any) (string, error)`
- `func GetHiveNamespace() string`
- `func EnsureRequeueAtLeastWithin(duration, result, err) (reconcile.Result, error)`
- `func MergeJsons(globalPullSecret, localPullSecret, cdLog) (string, error)`
- `func SetProxyEnvVars(podSpec, httpProxy, httpsProxy, noProxy)`
- `func SafeDelete(cl, ctx, obj) error`
- `func ProcessAssetTemplate(assetBytes, values) ([]byte, error)`
- `func ListRuntimeObjects(c, typesToList, opts...) ([]runtime.Object, error)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- all Hive CRD types and condition types
- `github.com/openshift/hive/apis/helpers` -- resource name generation
- `github.com/openshift/hive/pkg/constants` -- all constant keys, env vars, labels, annotations
- `github.com/openshift/installer/pkg/types/vsphere` -- VCenters metadata for credential validation
- `github.com/openshift/library-go/pkg/controller` -- EnsureOwnerRef
- `github.com/vmware/govmomi` -- vSphere SOAP/REST client for credential validation
- `github.com/prometheus/client_golang/prometheus` -- Prometheus metrics registration
- `k8s.io/api/batch/v1`, `k8s.io/api/apps/v1`, `k8s.io/api/core/v1`, `k8s.io/api/rbac/v1` -- Kubernetes resource types
- `k8s.io/client-go` -- rest config, flowcontrol, workqueue, clientcmd, cache
- `sigs.k8s.io/controller-runtime` -- client, controller, handler, reconcile, metrics, event, source
- `sigs.k8s.io/yaml` -- YAML unmarshaling

## Capabilities

- Centralized condition initialization, setting, and sorting for ClusterDeployment, ClusterClaim, ClusterPool, ClusterProvision, SyncSet, DNSZone, MachinePool, ClusterDeprovision, ClusterInstall
- Generic FindCondition using Go generics over any Hive condition type
- Metrics-instrumented Kubernetes client wrapper with per-controller request counters and histograms
- DelayingReconciler wrapper that handles 401/403 errors with backoff requeue
- Controller concurrency and rate limiter configuration via environment variables
- Expectations cache (ported from upstream Kubernetes) for tracking expected create/delete events
- Owner reference reconciliation ensuring correct controller references on derived objects
- Job and StatefulSet hash calculation for change detection
- StatefulSet ordinal-based UID assignment for sharded controllers
- vSphere credential validation against VCenter API (supports both flat and multi-vcenter secret shapes)
- Kubeconfig security validation (rejects exec-based auth)
- Error scrubbing of cloud provider messages (AWS request IDs, Azure error descriptions, certificate timestamps)
- Rate-limited event handler wrappers for typed and untyped events
- Service account, role, and role binding setup for install/uninstall jobs
- Taint deduplication and identification utilities
- DNS zone relocation logic (outgoing, incoming, complete states)
- Proxy environment variable propagation to pod specs

## Understanding Score

0.84

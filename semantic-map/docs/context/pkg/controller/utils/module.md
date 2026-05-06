<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/utils/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AWSHostedZoneRole`
- `AWSServiceProviderSecretName` — Generate the name of the Secret containing AWS Service Provider configuration. The prefix should be specified when the env var is known to refer to the global (hive namespace) Sec…
- `AddAdditionalKubeconfigCAs` — AddAdditionalKubeconfigCAs adds additional certificate authorities to a given kubeconfig
- `AddControllerMetricsTransportWrapper` — AddControllerMetricsTransportWrapper adds a transport wrapper to the given rest config which exposes metrics based on the requests being made.
- `AddFinalizer` — AddFinalizer adds a finalizer to the given object
- `AddLogFields`
- `AddLogFieldsEnvVar`
- `AdditionalLogFieldHavinThing`
- `AreAllConditionsInDesiredState` — AreAllConditionsInDesiredState checks if all cluster deployment conditions are in their desired state
- `AzureResourceGroup`
- `BuildControllerLogger` — BuildControllerLogger returns a logger for controllers with consistent fields.
- `CalculateJobSpecHash` — CalculateJobSpecHash returns a hash of the job.Spec.
- `CalculateStatefulSetSpecHash` — CalculateStatefulSetSpecHash returns a hash of the statefulset.Spec.
- `ClearRelocateAnnotation`
- `ClientBurstEnvVariableFormat`
- `ClientQPSEnvVariableFormat`
- `ConcurrentReconcilesEnvVariableFormat`
- `ControlleeExpectations` — ControlleeExpectations track controllee creates/deletes.
- `ControlleeExpectations.Add` — Add increments the add and del counters.
- `ControlleeExpectations.Fulfilled` — Fulfilled returns true if this expectation has been fulfilled.
- `ControlleeExpectations.GetExpectations` — GetExpectations returns the add and del expectations of the controllee.
- `ControllerMetricsTripper` — ControllerMetricsTripper is a RoundTripper implementation which tracks our metrics for client requests.
- `ControllerMetricsTripper.CancelRequest`
- `ControllerMetricsTripper.RoundTrip` — RoundTrip implements the http RoundTripper interface.
- `CopyAnnotations`
- `CopyLogAnnotation`
- `CopySecret` — CopySecret copies the secret defined by src to dest.  Returns true if an actual change was made.
- `CredentialsSecretName` — CredentialsSecretName returns the name of the credentials secret for platforms that have a CredentialsSecretRef. An empty string is returned if platform has none.
- `DNSZoneName` — DNSZoneName returns the predictable name for a DNSZone for the given ClusterDeployment.
- `DeleteFinalizer` — DeleteFinalizer removes a finalizer from the given object
- `Dotted` — Dotted adds a trailing dot to a domain if it doesn't exist.
- `EnqueueDNSZonesOwnedByClusterDeployment`
- `EnsureRequeueAtLeastWithin` — EnsureRequeueAtLeastWithin ensures that the requeue of the object will occur within the given duration. If the reconcile result and error will already result in a requeue soon eno…
- `ErrorScrub` — ErrorScrub scrubs cloud error messages destined for CRD status to remove things that change every attempt, such as request IDs, which subsequently cause an infinite update/reconci…
- `ExpKeyFunc` — ExpKeyFunc to parse out the key from a ControlleeExpectation
- `Expectations` — Expectations is a cache mapping controllers to what they expect to see before being woken up for a sync.
- `Expectations.CreationObserved` — CreationObserved atomically decrements the `add` expectation count of the given controller.
- `Expectations.DeleteExpectations` — DeleteExpectations deletes the expectations of the given controller from the TTLStore.
- `Expectations.DeletionObserved` — DeletionObserved atomically decrements the `del` expectation count of the given controller.
- `Expectations.ExpectCreations` — ExpectCreations sets the expectations to expect the specified number of additions for the controller with the specified key.
- `Expectations.ExpectDeletions` — ExpectDeletions sets the expectations to expect the specified number of deletions for the controller with the specified key.
- `Expectations.GetExpectations` — GetExpectations returns the ControlleeExpectations of the given controller.
- `Expectations.LowerExpectations` — LowerExpectations decrements the expectation counts of the given controller.
- `Expectations.RaiseExpectations` — RaiseExpectations increments the expectation counts of the given controller.
- `Expectations.SatisfiedExpectations` — SatisfiedExpectations returns true if the required adds/dels for the given controller have been observed. Add/del counts are established by the controller at sync time, and update…
- `Expectations.SetExpectations` — SetExpectations registers new expectations for the given controller. Forgets existing expectations.
- `ExpectationsInterface` — ExpectationsInterface is an interface that allows users to set and wait on expectations. Only abstracted out for testing. Warning: if using KeyFunc it is not safe to use a single …
- `ExpectationsTimeout`
- `ExtractLogFields` — ExtractLogFields knows where to look in an AdditionalLogFieldHavinThing for additional log fields. It attempts to extract them and parse them as a JSON string representing a map. …
- `FindCondition`
- `GCPNetworkProjectID`
- `GetChecksumOfObject` — GetChecksumOfObject returns the md5sum hash of the object passed in.
- `GetChecksumOfObjects` — GetChecksumOfObjects returns the md5sum hash of the objects passed in.
- `GetClusterPlatform` — GetClusterPlatform returns the platform of a given ClusterDeployment
- `GetControllerConfig`
- `GetDuckType` — GetDuckType uses the client to fetch a type defined by gck and key, and marshals it into the duck type defined in obj.
- `GetHiveNamespace` — GetHiveNamespace determines the namespace where core hive components run (hive-controllers, hiveadmission), by checking for the required environment variable.
- `GetMyOrdinalID` — GetMyOrdinalID expects to be executed in the context of a pod which is a replica of a StatefulSet. We thus expect the pod name to have an ordinal suffix ("-0", "-1", etc) by which…
- `GetUniqueTaints` — GetUniqueTaints collapses the duplicates from the list of taints provided before returning them. Key+Effect are unique identifiers, and the first Value of the taint entry encounte…
- `HasFinalizer` — HasFinalizer returns true if the given object has the given finalizer
- `IdentifierForTaint`
- `InfraDisabled` — InfraDisabled answers whether cd has requested to disable controllers/codepaths that talk to the cloud infrastructure. Currently only works for Azure; always returns false for oth…
- `InitializeClusterClaimConditions` — InitializeClusterClaimConditions initializes the given set of conditions for the first time, set with Status Unknown. If the conditions already exist, they are not affected. The f…
- `InitializeClusterDeploymentConditions` — InitializeClusterDeploymentConditions initializes the given set of conditions for the first time, set with Status Unknown. If the conditions already exist, they are not affected. …
- `InitializeClusterPoolConditions` — InitializeClusterPoolConditions initializes the given set of conditions for the first time, set with Status Unknown. If the conditions already exist, they are not affected. The fi…
- `InitializeMachinePoolConditions` — InitializeMachinePoolConditions initializes the given set of conditions for the first time, set with Status Unknown
- `InjectWatcher` — InjectWatcher injects the reconciler that implements the watcherInjectable interface with the provided controller. If the reconciler passed does not implement the interface, it re…
- `InstallServiceAccountName`
- `IsClusterDeploymentErrorUpdateEvent` — IsClusterDeploymentErrorUpdateEvent returns true when the update event is from error state in clusterdeployment.
- `IsClusterMarkedForRemoval` — IsClusterMarkedForRemoval returns true when the hive.openshift.io/remove-cluster-from-pool annotation is set to true value in the clusterdeployment
- `IsClusterPausedOrRelocating` — IsClusterPausedOrRelocating checks if the syncing to the cluster is paused or if the cluster is relocating
- `IsConditionInDesiredState` — IsConditionInDesiredState checks if the condition status is in its desired/expected state
- `IsConditionWithPositivePolarity` — IsConditionWithPositivePolarity checks if cluster deployment condition has positive polarity
- `IsDeadlineExceeded` — IsDeadlineExceeded returns true if the job failed due to deadline being exceeded
- `IsDeleteProtected`
- `IsFailed` — IsFailed returns true if the job failed
- `IsFakeCluster`
- `IsFinished` — IsFinished returns true if the job completed (succeeded or failed)
- `IsRelocating`
- `IsSuccessful` — IsSuccessful returns true if the job was successful


*(Showing first 80 of 137 merged export names.)*

_(At least one package hit the **`export_entries`** synopsis cap; see **`export_entry_total`** / **`export_entries_truncated`**.)_

## Internal Dependencies

- `bytes`
- `context`
- `crypto/md5`
- `encoding/hex`
- `encoding/json`
- `errors`
- `fmt`
- `github.com/openshift/hive/apis/helpers`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/installer/pkg/types/vsphere`
- `github.com/openshift/library-go/pkg/controller`
- `github.com/pkg/errors`
- `github.com/prometheus/client_golang/prometheus`
- `github.com/sirupsen/logrus`
- `github.com/vmware/govmomi/vapi/rest`
- `github.com/vmware/govmomi/vim25`
- `github.com/vmware/govmomi/vim25/soap`
- `golang.org/x/time/rate`
- `io`
- `k8s.io/api/apps/v1`
- `k8s.io/api/batch/v1`
- `k8s.io/api/core/v1`
- `k8s.io/api/rbac/v1`
- `k8s.io/apimachinery/pkg/api/equality`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/api/meta`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/errors`
- `k8s.io/apimachinery/pkg/util/rand`
- `k8s.io/apimachinery/pkg/util/sets`
- `k8s.io/client-go/rest`
- `k8s.io/client-go/tools/cache`
- `k8s.io/client-go/tools/clientcmd`
- `k8s.io/client-go/tools/clientcmd/api`
- `k8s.io/client-go/util/flowcontrol`
- `k8s.io/client-go/util/workqueue`
- `k8s.io/utils/clock`
- `math/big`
- `net/http`
- `net/url`
- `os`
- `reflect`
- `regexp`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/apiutil`
- `sigs.k8s.io/controller-runtime/pkg/controller`
- `sigs.k8s.io/controller-runtime/pkg/controller/controllerutil`
- `sigs.k8s.io/controller-runtime/pkg/event`
- `sigs.k8s.io/controller-runtime/pkg/handler`
- `sigs.k8s.io/controller-runtime/pkg/manager`
- `sigs.k8s.io/controller-runtime/pkg/metrics`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`
- `sigs.k8s.io/controller-runtime/pkg/source`
- `sigs.k8s.io/yaml`
- `slices`
- `sort`
- `strconv`
- `strings`
- `sync/atomic`
- `text/template`
- `time`

## Capabilities

- **`package`** name(s): **utils**.
- Go **`import`** edges listed below (66 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/utils`.

## Understanding Score

0.0

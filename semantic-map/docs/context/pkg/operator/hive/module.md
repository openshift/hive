# Module: pkg/operator/hive

## Responsibility

Implements the core HiveConfig reconciler -- the operator controller that reads the HiveConfig CR and deploys/configures all Hive components into the target namespace. This is the heart of the Hive operator. It reconciles the desired state declared in HiveConfig into actual Kubernetes resources: Deployments (hive-controllers, hiveadmission), StatefulSets (clustersync, machinepool), Services, ServiceAccounts, RBAC, ValidatingWebhookConfigurations, APIService, ConfigMaps (managed domains, private link, failed provision, metrics, feature gates, supported contracts, controllers config), and Secrets (additional CAs, image pull secrets).

## Public Interface/API

- `Add(mgr manager.Manager) error` -- creates the HiveConfig controller and adds it to the Manager. Sets up watches on HiveConfig, Deployments, DaemonSets, Services, StatefulSets, CRDs (for contract discovery), Namespaces, Proxy (OpenShift), APIServer (OpenShift), ConfigMaps (aggregator CA), and Secrets (serving cert).
- `ControllerName` -- constant `hivev1.HiveControllerName`.
- `HiveOperatorNamespaceEnvVar` -- `"HIVE_OPERATOR_NS"`.
- `HiveConfigConditions` -- slice of condition types managed by this controller (currently just `HiveReadyCondition`).
- `ReconcileHiveConfig` -- the reconciler struct:
  - `Reconcile(ctx, request) (reconcile.Result, error)` -- main reconcile loop. Ensures target namespace, deploys ConfigMaps, deploys hive-controllers Deployment, deploys clustersync/machinepool StatefulSets, deploys hiveadmission (Deployment + APIService + webhooks), cleans up previous target namespaces, manages HiveConfig conditions.
  - `Get(ctx, nsName, obj) error` -- dynamic client wrapper (bypasses cache issues).
  - `List(ctx, obj, namespace, listOpts) error` -- dynamic client wrapper.
- `GetHiveNamespace(config) string` -- returns the target namespace from HiveConfig (defaults to `"hive"`).
- `SetHiveConfigCondition(conditions, type, status, reason, message) []HiveConfigCondition` -- sets or updates a condition.
- `FindHiveConfigCondition(conditions, type) *HiveConfigCondition` -- finds a condition by type.
- `InitializeHiveConfigConditions(existing, toAdd) ([]HiveConfigCondition, bool)` -- initializes conditions with Unknown status.

## Key Internal Workflows

1. **Hive Controllers Deployment** (`deployHive`): Loads the deployment asset from bindata, configures container image/pull policy/env vars (disabled controllers, log level, sync intervals, proxy, Velero, ArgoCD, delete protection, release image verification), mounts config volumes, includes additional CAs, applies RBAC, handles OpenShift-specific assets.
2. **Hive Admission Deployment** (`deployHiveAdmission`): Deploys the admission webhook server with TLS configuration (discovers TLS min version and cipher suites from OpenShift APIServer config), injects CA certs into APIService and webhook configurations, handles serving cert rotation via hash annotations.
3. **Sharded Controller StatefulSets** (`deployStatefulSet`): Deploys clustersync and machinepool as separate StatefulSets with configurable replicas/resources. Detects spec changes via hash annotations and deletes/recreates StatefulSets when immutable fields change.
4. **ConfigMap Management** (`deployConfigMap`): Creates/updates ConfigMaps for managed domains, AWS/generic private link, failed provision config, metrics config, supported contracts (discovered from CRDs with contract labels), controllers config (concurrency, QPS, burst), and feature gates.
5. **Dynamic Client** (`dynamicclient.go`): Custom `Get`/`List` methods using `k8s.io/client-go/dynamic` to bypass controller-runtime cache issues with HiveConfig and other global resources.
6. **Apply Framework** (`apply.go`): Generic framework for loading assets from bindata, applying namespace overrides, setting owner references for garbage collection, and applying/deleting runtime objects via the resource helper.

## Internal Dependencies

Key Hive-internal imports:
- `apis/hive/v1`, `apis/hivecontracts/v1alpha1` -- HiveConfig, controller names, deployment names, contract types
- `pkg/constants` -- namespace defaults, env var names, feature gate vars
- `pkg/controller/images` -- hive image env vars
- `pkg/controller/utils` -- proxy vars, shared pod config, statefulset hash, asset template processing
- `pkg/operator/assets` -- embedded YAML manifests
- `pkg/operator/metrics` -- ReconcileObserver for timing
- `pkg/resource` -- resource Helper for apply/create/delete operations
- `pkg/util/contracts` -- SupportedContractImplementations types
- `pkg/util/logrus` -- LoggingEventRecorder
- `pkg/util/scheme` -- shared scheme

Key external:
- `github.com/openshift/api/config/v1`, `github.com/openshift/api/apps/v1` -- OpenShift Proxy, APIServer, DeploymentConfig types
- `github.com/openshift/library-go/pkg/operator/...` -- TLS config observation, resource reading, resource sync
- `k8s.io/kube-aggregator/pkg/apis/apiregistration/v1` -- APIService type
- `k8s.io/api/admissionregistration/v1` -- webhook configuration types
- `k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1` -- CRD type for contract discovery

## Capabilities

- **Package**: `hive`
- Source files: `hive_controller.go` (controller setup + reconcile, ~843 lines), `hive.go` (deployHive, ~595 lines), `hiveadmission.go` (deployHiveAdmission + cert injection, ~431 lines), `sharded_controllers.go` (StatefulSet deployment, ~284 lines), `configmap.go` (ConfigMap management, ~378 lines), `conditions.go` (condition helpers, ~76 lines), `apply.go` (apply framework, ~178 lines), `dynamicclient.go` (dynamic client wrappers, ~82 lines), `operatorutils.go` (utility functions, ~112 lines).
- 67 unique import paths.

## Understanding Score

0.83

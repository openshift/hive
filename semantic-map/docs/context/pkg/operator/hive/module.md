# Module atlas

## Responsibility

Implements the Hive operator's main reconciler for the HiveConfig custom resource. Deploys and manages all Hive components (controllers, hiveadmission webhooks, sharded controllers like clustersync and machinepool) based on the desired state in HiveConfig. Handles namespace management, configmap generation, certificate/CA aggregation, and resource garbage collection.

## Public Interface/API

- `Add(mgr manager.Manager) error` -- creates the HiveConfig controller and adds it to the manager
- `ControllerName` const -- `hivev1.HiveControllerName`
- `HiveOperatorNamespaceEnvVar` const -- `"HIVE_OPERATOR_NS"`
- `ReconcileHiveConfig` struct -- reconciler with `Reconcile(ctx, request) (reconcile.Result, error)`, `Get(...)`, `List(...)`
- `HiveConfigConditions` var -- list of condition types controlled by this controller
- `SetHiveConfigCondition(conditions, conditionType, status, reason, message) []HiveConfigCondition`
- `FindHiveConfigCondition(conditions, conditionType) *HiveConfigCondition`
- `InitializeHiveConfigConditions(existingConditions, conditionsToBeAdded) ([]HiveConfigCondition, bool)`
- `GetHiveNamespace(config *hivev1.HiveConfig) string`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `hivecontracts/v1alpha1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/images`, `pkg/controller/utils`
- `github.com/openshift/hive/pkg/operator/assets`, `pkg/operator/metrics`
- `github.com/openshift/hive/pkg/resource`
- `github.com/openshift/hive/pkg/util/contracts`, `pkg/util/logrus`, `pkg/util/scheme`
- `github.com/openshift/library-go/pkg/operator/...` (configobserver, resourceread, resourcesynccontroller)
- `github.com/openshift/api/config/v1`, `github.com/openshift/api/apps/v1`
- `k8s.io/client-go/dynamic`, `discovery`, `informers`, `kubernetes`
- `sigs.k8s.io/controller-runtime/pkg/controller`, `reconcile`, `manager`

## Capabilities

- Reconciles HiveConfig CR to deploy/update hive-controllers Deployment, hiveadmission webhooks and APIService
- Manages sharded controllers (clustersync, machinepool) as StatefulSets
- Generates and manages configmaps for controller configuration (managed domains, failure domains, contracts, install log regexes)
- Aggregates additional CA certificates into a combined secret
- Deploys RBAC roles, role bindings, and service accounts from embedded assets
- Cleans up resources from previous target namespaces on namespace changes
- Sets HiveConfig conditions (HiveReady)
- Handles OpenShift-specific integration (proxy config, aggregator CA, service CA)

## Understanding Score

0.85

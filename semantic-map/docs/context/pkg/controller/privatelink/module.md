# Module atlas

## Responsibility

Reconciles private link / private service connect resources for ClusterDeployments. Manages the lifecycle of cloud private link infrastructure (VPC endpoints for AWS, Private Service Connect for GCP) by creating actuators per cloud platform and coordinating setup during provisioning and cleanup during deprovisioning.

## Public Interface/API

- `ControllerName` -- constant `hivev1.PrivateLinkControllerName`
- `Add(mgr manager.Manager) error` -- creates controller with watches on ClusterDeployment, ClusterProvision, ClusterDeprovision
- `AddToManager(mgr, r, concurrentReconciles, rateLimiter) error`
- `PrivateLinkReconciler` -- reconciler struct embedding `client.Client`
- `PrivateLinkReconciler.Reconcile(ctx, request) (reconcile.Result, error)` -- main reconcile dispatching to platform-specific actuators
- `PrivateLink` -- struct holding hub and link actuators for a specific ClusterDeployment
- `PrivateLink.Reconcile(privateLinkEnabled bool) (reconcile.Result, error)` -- orchestrates private link lifecycle
- `CreateActuator(client, cloud, actuatorType, config, cd, ...) (actuator.Actuator, error)` -- factory for cloud-specific actuators
- `ReadPrivateLinkControllerConfigFile() (*hivev1.PrivateLinkConfig, error)` -- reads config from env-pointed file

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/hive/pkg/controller/metrics` -- ReconcileObserver
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface, ActuatorType
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator` -- AWS actuator factory
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator` -- GCP actuator factory
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- condition helpers
- `github.com/openshift/hive/pkg/controller/utils` -- controller config, finalizers
- `sigs.k8s.io/controller-runtime` -- controller, reconcile, manager

## Capabilities

- Watches ClusterDeployment, ClusterProvision, and ClusterDeprovision resources
- Creates hub and link actuators per cloud platform (AWS, GCP) from HiveConfig private link settings
- Manages `hive.openshift.io/private-link` finalizer for cleanup
- Coordinates private link setup during provisioning (waits for ClusterProvision with API URL)
- Handles cleanup on ClusterDeployment deletion or when private link is disabled
- Supports sync rate limiting to reduce cloud API usage
- Reads controller configuration from file specified via environment variable

## Understanding Score

0.85

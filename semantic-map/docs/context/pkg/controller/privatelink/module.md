# Module atlas

## Responsibility

Provides a cloud-agnostic PrivateLink controller that manages private connectivity to cluster API servers. This is the newer, multi-cloud version of the AWS-specific `awsprivatelink` controller. It delegates cloud-specific operations to actuator implementations (AWS hub actuator, GCP link actuator) selected based on the ClusterDeployment's platform. The controller handles the full lifecycle: creation, reconciliation, and cleanup of private link resources.

## Public Interface/API

- `Add` -- Creates the controller and adds it to the Manager.
- `AddToManager` -- Adds the controller to a manager.
- `ControllerName` -- Constant referencing `hivev1.PrivateLinkControllerName`.
- `PrivateLinkReconciler` -- Main reconciler struct with actuator factory.
- `PrivateLinkReconciler.Reconcile` -- Main reconcile loop, delegates to actuator.
- `PrivateLink` -- Wrapper struct that holds CD and actuator, with its own `Reconcile` method.
- `CreateActuator` -- Factory function that creates an actuator based on cloud platform and config.
- `ReadPrivateLinkControllerConfigFile` -- Reads private link controller configuration from file.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterProvision, ClusterDeprovision CRs.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/awsactuator` -- AWS hub actuator.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator/gcpactuator` -- GCP link actuator.
- `github.com/openshift/hive/pkg/controller/privatelink/conditions` -- Shared condition management.
- `github.com/openshift/hive/pkg/controller/utils` -- Finalizers, conditions, controller config.
- `github.com/openshift/hive/pkg/controller/metrics` -- Reconcile time observation.

## Capabilities

- Reconciles: **ClusterDeployment** (with private link enabled).
- Watches: ClusterDeployment, ClusterProvision (owner), ClusterDeprovision (owner).
- Conditions set: `PrivateLinkFailedClusterDeploymentCondition`, `PrivateLinkReadyClusterDeploymentCondition` (via conditions package).
- Finalizer: `hive.openshift.io/private-link`.
- Key logic: Selects actuator based on platform (AWS or GCP), delegates reconcile/cleanup. Handles provision reattempt cleanup, shouldSync() rate limiting, and condition management.

## Understanding Score

0.80

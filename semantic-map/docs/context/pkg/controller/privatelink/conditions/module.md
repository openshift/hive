# Module atlas

## Responsibility

Shared condition management utilities for the private link controller and its actuators. Provides functions to initialize, set error, and set ready conditions on ClusterDeployments for PrivateLink, with retry support for conflict resolution.

## Public Interface/API

- `InitializeConditions` -- Initializes `PrivateLinkFailedClusterDeploymentCondition` and `PrivateLinkReadyClusterDeploymentCondition`.
- `SetErrCondition` -- Sets the failed condition to true and ready to false.
- `SetErrConditionWithRetry` -- Same as SetErrCondition but with retry-on-conflict.
- `SetReadyCondition` -- Sets the ready condition and optionally clears the failed condition.
- `SetReadyConditionWithRetry` -- Same as SetReadyCondition but with retry-on-conflict.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, condition types.
- `github.com/openshift/hive/pkg/controller/utils` -- Condition helpers (SetClusterDeploymentConditionWithChangeCheck, FindCondition, ErrorScrub).

## Capabilities

Pure utility package. Manages two conditions: `PrivateLinkFailedClusterDeploymentCondition` and `PrivateLinkReadyClusterDeploymentCondition`. Used by both the privatelink controller and its actuator implementations.

## Understanding Score

0.90

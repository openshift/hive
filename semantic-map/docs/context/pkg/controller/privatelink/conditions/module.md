# Module atlas

## Responsibility

Provides helper functions for managing private link-related conditions (`PrivateLinkFailed`, `PrivateLinkReady`) on ClusterDeployment resources, with retry-on-conflict support for status updates.

## Public Interface/API

- `InitializeConditions(cd *hivev1.ClusterDeployment) ([]hivev1.ClusterDeploymentCondition, bool)` -- initializes private link conditions if not present
- `SetErrCondition(client, cd, reason, err, logger) error` -- sets PrivateLinkFailed=True and PrivateLinkReady=False
- `SetErrConditionWithRetry(client, cd, reason, err, logger) error` -- SetErrCondition with retry-on-conflict
- `SetReadyCondition(client, cd, completed, reason, message, logger) error` -- sets PrivateLinkReady to specified status, clears PrivateLinkFailed on success
- `SetReadyConditionWithRetry(client, cd, completed, reason, message, logger) error` -- SetReadyCondition with retry-on-conflict

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, condition types
- `github.com/openshift/hive/pkg/controller/utils` -- SetClusterDeploymentConditionWithChangeCheck, FindCondition, InitializeClusterDeploymentConditions
- `k8s.io/apimachinery/pkg/util/wait`, `k8s.io/client-go/util/retry` -- retry-on-conflict
- `sigs.k8s.io/controller-runtime/pkg/client` -- client for status updates

## Capabilities

- Initializes `PrivateLinkFailed` and `PrivateLinkReady` conditions on ClusterDeployment
- Sets error conditions with scrubbed error messages
- Sets ready conditions with special handling: allows transition to Ready but prevents demotion once Ready
- All condition setters re-fetch the ClusterDeployment before update to minimize conflicts
- Retry variants use exponential backoff for conflict resolution

## Understanding Score

0.9

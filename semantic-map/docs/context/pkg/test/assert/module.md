# Module atlas

## Responsibility

Test assertion helpers for Hive controller tests. Provides functions to validate ClusterDeployment conditions, ClusterDeploymentCustomization conditions, container environment variables, time windows, and structural equality of runtime objects (ignoring ResourceVersion and TypeMeta).

## Public Interface/API

- `AssertAllContainersHaveEnvVar(t, podSpec, key, value)` -- asserts every container in a PodSpec has exactly one occurrence of the specified env var with the expected value
- `AssertCDCConditions(t, cdc, expectedConditions)` -- asserts expected `metav1.Condition` entries are present on a ClusterDeploymentCustomization with matching status, reason, and optionally message
- `AssertConditionStatus(t, cd, condType, status)` -- asserts a single ClusterDeployment condition exists with the expected status
- `AssertConditions(t, cd, expectedConditions)` -- asserts expected `ClusterDeploymentCondition` entries are present on a ClusterDeployment with matching status, reason, and optionally message
- `AssertEqualWhereItCounts(t, x, y, msg)` -- deep-compares two `runtime.Object` values, ignoring ResourceVersion and TypeMeta
- `BetweenTimes(t, actual, startTime, endTime, msgAndArgs...)` -- asserts a time falls within an inclusive window

## Internal Dependencies

- `fmt`, `testing`, `time`
- `github.com/google/go-cmp/cmp`
- `github.com/stretchr/testify/assert`
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, ClusterDeploymentCustomization condition types
- `k8s.io/api/core/v1` -- PodSpec, ConditionStatus
- `k8s.io/apimachinery/pkg/apis/meta/v1` -- Condition, Object
- `k8s.io/apimachinery/pkg/runtime`, `k8s.io/apimachinery/pkg/runtime/schema`

## Capabilities

- **Package**: `assert`
- Not a builder package; provides assertion utilities only
- Uses `testify/assert` for failure reporting and `go-cmp` for structural diffs

## Understanding Score

0.9

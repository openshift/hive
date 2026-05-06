# Module atlas

## Responsibility

Test assertion helpers for validating Hive CRD conditions, container environment variables, object equality (ignoring ResourceVersion/TypeMeta), and time-window checks.

## Public Interface/API

- `BetweenTimes(t *testing.T, actual, startTime, endTime time.Time, msgAndArgs ...any) bool` -- asserts a time falls within an inclusive window
- `AssertAllContainersHaveEnvVar(t *testing.T, podSpec *corev1.PodSpec, key, value string)` -- asserts every container in a PodSpec has the given env var with the given value
- `AssertConditionStatus(t *testing.T, cd *hivev1.ClusterDeployment, condType hivev1.ClusterDeploymentConditionType, status corev1.ConditionStatus)` -- asserts a condition exists with expected status
- `AssertConditions(t *testing.T, cd *hivev1.ClusterDeployment, expectedConditions []hivev1.ClusterDeploymentCondition)` -- asserts expected conditions are present with matching status, reason, and optionally message
- `AssertCDCConditions(t *testing.T, cdc *hivev1.ClusterDeploymentCustomization, expectedConditions []metav1.Condition)` -- same as AssertConditions but for ClusterDeploymentCustomization using metav1.Condition
- `AssertEqualWhereItCounts(t *testing.T, x, y runtime.Object, msg string)` -- compares two runtime.Objects ignoring ResourceVersion and TypeMeta

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/google/go-cmp/cmp`
- `github.com/stretchr/testify/assert`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`

## Capabilities

- Condition validation on ClusterDeployment and ClusterDeploymentCustomization resources
- Container env var presence and uniqueness validation on PodSpecs
- Deep equality comparison of runtime.Objects with metadata scrubbing
- Time-window boundary assertions

## Understanding Score

0.85

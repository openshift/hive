<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/test/assert/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `AssertAllContainersHaveEnvVar`
- `AssertCDCConditions` — AssertConditions asserts if the expected conditions are present on the cluster deployment. It also asserts if those conditions have the expected status, reason, and (optionally) m…
- `AssertConditionStatus` — AssertConditionStatus asserts if a condition is present on the cluster deployment and has the expected status
- `AssertConditions` — AssertConditions asserts if the expected conditions are present on the cluster deployment. It also asserts if those conditions have the expected status, reason, and (optionally) m…
- `AssertEqualWhereItCounts` — AssertEqualWhereItCounts compares two runtime.Objects, ignoring their ResourceVersion and TypeMeta, asserting that they are otherwise equal. This and cleanRVAndTypeMeta were borro…
- `BetweenTimes` — BetweenTimes asserts that the time is within the time window, inclusive of the start and end times.

## Internal Dependencies

- `fmt`
- `github.com/google/go-cmp/cmp`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/stretchr/testify/assert`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/apis/meta/v1`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `testing`
- `time`

## Capabilities

- **`package`** name(s): **assert**.
- Go **`import`** edges listed below (10 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/test/assert`.

## Understanding Score

0.0

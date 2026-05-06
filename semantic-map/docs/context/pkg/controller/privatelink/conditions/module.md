<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/privatelink/conditions/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `InitializeConditions`
- `SetErrCondition`
- `SetErrConditionWithRetry`
- `SetReadyCondition`
- `SetReadyConditionWithRetry`

## Internal Dependencies

- `context`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/utils`
- `github.com/pkg/errors`
- `github.com/sirupsen/logrus`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/types`
- `k8s.io/apimachinery/pkg/util/wait`
- `k8s.io/client-go/util/retry`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `time`

## Capabilities

- **`package`** name(s): **conditions**.
- Go **`import`** edges listed below (11 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/privatelink/conditions`.

## Understanding Score

0.0

<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/privatelink/actuator/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockActuator` — MockActuator is a mock of Actuator interface.
- `MockActuator.Cleanup` — Cleanup mocks base method.
- `MockActuator.CleanupRequired` — CleanupRequired mocks base method.
- `MockActuator.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockActuator.Reconcile` — Reconcile mocks base method.
- `MockActuator.ShouldSync` — ShouldSync mocks base method.
- `MockActuatorMockRecorder` — MockActuatorMockRecorder is the mock recorder for MockActuator.
- `MockActuatorMockRecorder.Cleanup` — Cleanup indicates an expected call of Cleanup.
- `MockActuatorMockRecorder.CleanupRequired` — CleanupRequired indicates an expected call of CleanupRequired.
- `MockActuatorMockRecorder.Reconcile` — Reconcile indicates an expected call of Reconcile.
- `MockActuatorMockRecorder.ShouldSync` — ShouldSync indicates an expected call of ShouldSync.

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator`
- `github.com/sirupsen/logrus`
- `go.uber.org/mock/gomock`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (6 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/privatelink/actuator/mock`.

## Understanding Score

0.0

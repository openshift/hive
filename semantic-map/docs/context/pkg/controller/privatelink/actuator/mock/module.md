# Module atlas

## Responsibility

Mockgen-generated mock of the `actuator.Actuator` interface for testing private link controller logic.

## Public Interface/API

- `MockActuator` -- mock of `Actuator` interface (generated from `actuator.go`)
- `MockActuator.Cleanup`, `MockActuator.CleanupRequired`, `MockActuator.Reconcile`, `MockActuator.ShouldSync` -- mock base methods
- `MockActuator.EXPECT()` -- returns `MockActuatorMockRecorder`
- `MockActuatorMockRecorder` -- recorder with expectation methods for all Actuator methods
- `NewMockActuator(ctrl *gomock.Controller) *MockActuator`

## Internal Dependencies

- `go.uber.org/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/controller/privatelink/actuator`
- `sigs.k8s.io/controller-runtime/pkg/reconcile`

## Capabilities

- Generated mock for `actuator.Actuator` interface enabling unit tests of private link controller reconciliation

## Understanding Score

0.7

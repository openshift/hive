# Module atlas

## Responsibility

Mockgen-generated mock of the `machinepool.Actuator` interface for testing MachinePool controller logic.

## Public Interface/API

- `MockActuator` -- mock of `Actuator` interface (generated from `actuator.go`)
- `MockActuator.GenerateMachineSets` -- mocks base method
- `MockActuator.EXPECT()` -- returns `MockActuatorMockRecorder`
- `MockActuatorMockRecorder.GenerateMachineSets` -- expectation method
- `NewMockActuator(ctrl *gomock.Controller) *MockActuator`

## Internal Dependencies

- `go.uber.org/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/api/machine/v1beta1`

## Capabilities

- Generated mock for `machinepool.Actuator` interface enabling unit tests of MachinePool reconciliation

## Understanding Score

0.7

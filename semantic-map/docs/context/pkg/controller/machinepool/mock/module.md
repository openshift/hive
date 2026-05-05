# Module atlas

## Responsibility

Generated gomock mock of the `machinepool.Actuator` interface from the parent package. Used in unit tests for the machinepool controller.

## Public Interface/API

- `MockActuator` -- Mock of `Actuator` interface with `GenerateMachineSets` method.
- `MockActuatorMockRecorder` -- Mock recorder for setting expectations.

## Internal Dependencies

- `github.com/golang/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment, MachinePool types used in mock method signatures.

## Capabilities

Generated mock file (`actuator_generated.go`).

## Understanding Score

0.95

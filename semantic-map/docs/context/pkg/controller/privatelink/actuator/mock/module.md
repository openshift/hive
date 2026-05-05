# Module atlas

## Responsibility

Generated gomock mock of the `actuator.Actuator` interface from the parent package. Used in unit tests for the private link controller.

## Public Interface/API

- `MockActuator` -- Mock of `Actuator` interface with `Cleanup`, `CleanupRequired`, `Reconcile`, `ShouldSync` methods.
- `MockActuatorMockRecorder` -- Mock recorder for setting expectations.

## Internal Dependencies

- `github.com/golang/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment types used in mock method signatures.
- `github.com/openshift/hive/pkg/controller/privatelink/actuator` -- Actuator interface being mocked.

## Capabilities

Generated mock file (`actuator_generated.go`).

## Understanding Score

0.95

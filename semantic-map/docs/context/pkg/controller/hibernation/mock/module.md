# Module atlas

## Responsibility

Generated gomock mocks of the `HibernationActuator`, `HibernationPreemptibleMachines`, and `csrHelper` interfaces from the parent hibernation package. Used in unit tests.

## Public Interface/API

- `MockHibernationActuator` -- Mock of `HibernationActuator` interface (CanHandle, StopMachines, StartMachines, MachinesRunning, MachinesStopped).
- `MockHibernationPreemptibleMachines` -- Mock of `HibernationPreemptibleMachines` interface (ReplaceMachines).
- `MockcsrHelper` -- Mock of internal `csrHelper` interface (Parse, Authorize, IsApproved, Approve).

## Internal Dependencies

- `github.com/golang/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1` -- ClusterDeployment types used in mock method signatures.

## Capabilities

Generated mock files (`hibernation_actuator_generated.go`, `csr_helper_generated.go`).

## Understanding Score

0.95

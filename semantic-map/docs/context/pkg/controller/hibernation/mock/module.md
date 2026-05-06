# Module atlas

## Responsibility

Mockgen-generated mocks of the `HibernationActuator`, `HibernationPreemptibleMachines`, and `csrHelper` interfaces for testing the hibernation controller.

## Public Interface/API

- `MockHibernationActuator` -- mock of `HibernationActuator` interface (generated from `hibernation_actuator.go`)
  - Methods: `CanHandle`, `StopMachines`, `StartMachines`, `MachinesRunning`, `MachinesStopped`, `EXPECT`
- `MockHibernationPreemptibleMachines` -- mock of `HibernationPreemptibleMachines` interface
  - Methods: `ReplaceMachines`, `EXPECT`
- `MockcsrHelper` -- mock of `csrHelper` interface (generated from `csr_helper.go`)
  - Methods: `Parse`, `Authorize`, `IsApproved`, `Approve`, `EXPECT`
- Corresponding `MockRecorder` types for each mock

## Internal Dependencies

- `go.uber.org/mock/gomock`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/api/machine/v1beta1`
- `k8s.io/api/certificates/v1`
- `k8s.io/client-go/kubernetes`

## Capabilities

- Generated mocks for hibernation controller unit tests covering cloud actuator operations, preemptible machine handling, and CSR approval logic

## Understanding Score

0.7

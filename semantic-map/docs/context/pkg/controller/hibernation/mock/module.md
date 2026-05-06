<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/hibernation/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockHibernationActuator` — MockHibernationActuator is a mock of HibernationActuator interface.
- `MockHibernationActuator.CanHandle` — CanHandle mocks base method.
- `MockHibernationActuator.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockHibernationActuator.MachinesRunning` — MachinesRunning mocks base method.
- `MockHibernationActuator.MachinesStopped` — MachinesStopped mocks base method.
- `MockHibernationActuator.StartMachines` — StartMachines mocks base method.
- `MockHibernationActuator.StopMachines` — StopMachines mocks base method.
- `MockHibernationActuatorMockRecorder` — MockHibernationActuatorMockRecorder is the mock recorder for MockHibernationActuator.
- `MockHibernationActuatorMockRecorder.CanHandle` — CanHandle indicates an expected call of CanHandle.
- `MockHibernationActuatorMockRecorder.MachinesRunning` — MachinesRunning indicates an expected call of MachinesRunning.
- `MockHibernationActuatorMockRecorder.MachinesStopped` — MachinesStopped indicates an expected call of MachinesStopped.
- `MockHibernationActuatorMockRecorder.StartMachines` — StartMachines indicates an expected call of StartMachines.
- `MockHibernationActuatorMockRecorder.StopMachines` — StopMachines indicates an expected call of StopMachines.
- `MockHibernationPreemptibleMachines` — MockHibernationPreemptibleMachines is a mock of HibernationPreemptibleMachines interface.
- `MockHibernationPreemptibleMachines.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockHibernationPreemptibleMachines.ReplaceMachines` — ReplaceMachines mocks base method.
- `MockHibernationPreemptibleMachinesMockRecorder` — MockHibernationPreemptibleMachinesMockRecorder is the mock recorder for MockHibernationPreemptibleMachines.
- `MockHibernationPreemptibleMachinesMockRecorder.ReplaceMachines` — ReplaceMachines indicates an expected call of ReplaceMachines.
- `MockcsrHelper` — MockcsrHelper is a mock of csrHelper interface.
- `MockcsrHelper.Approve` — Approve mocks base method.
- `MockcsrHelper.Authorize` — Authorize mocks base method.
- `MockcsrHelper.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockcsrHelper.IsApproved` — IsApproved mocks base method.
- `MockcsrHelper.Parse` — Parse mocks base method.
- `MockcsrHelperMockRecorder` — MockcsrHelperMockRecorder is the mock recorder for MockcsrHelper.
- `MockcsrHelperMockRecorder.Approve` — Approve indicates an expected call of Approve.
- `MockcsrHelperMockRecorder.Authorize` — Authorize indicates an expected call of Authorize.
- `MockcsrHelperMockRecorder.IsApproved` — IsApproved indicates an expected call of IsApproved.
- `MockcsrHelperMockRecorder.Parse` — Parse indicates an expected call of Parse.

## Internal Dependencies

- `crypto/x509`
- `github.com/openshift/api/machine/v1beta1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/sirupsen/logrus`
- `go.uber.org/mock/gomock`
- `k8s.io/api/certificates/v1`
- `k8s.io/client-go/kubernetes`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (9 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/hibernation/mock`.

## Understanding Score

0.0

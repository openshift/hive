<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/machinepool/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockActuator` — MockActuator is a mock of Actuator interface.
- `MockActuator.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockActuator.GenerateMachineSets` — GenerateMachineSets mocks base method.
- `MockActuatorMockRecorder` — MockActuatorMockRecorder is the mock recorder for MockActuator.
- `MockActuatorMockRecorder.GenerateMachineSets` — GenerateMachineSets indicates an expected call of GenerateMachineSets.

## Internal Dependencies

- `github.com/openshift/api/machine/v1beta1`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/sirupsen/logrus`
- `go.uber.org/mock/gomock`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (5 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/machinepool/mock`.

## Understanding Score

0.0

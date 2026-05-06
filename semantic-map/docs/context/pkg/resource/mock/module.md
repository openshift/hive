<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/resource/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockHelper` — MockHelper is a mock of Helper interface.
- `MockHelper.Apply` — Apply mocks base method.
- `MockHelper.ApplyRuntimeObject` — ApplyRuntimeObject mocks base method.
- `MockHelper.Create` — Create mocks base method.
- `MockHelper.CreateOrUpdate` — CreateOrUpdate mocks base method.
- `MockHelper.CreateOrUpdateRuntimeObject` — CreateOrUpdateRuntimeObject mocks base method.
- `MockHelper.CreateRuntimeObject` — CreateRuntimeObject mocks base method.
- `MockHelper.Delete` — Delete mocks base method.
- `MockHelper.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockHelper.Info` — Info mocks base method.
- `MockHelper.Patch` — Patch mocks base method.
- `MockHelperMockRecorder` — MockHelperMockRecorder is the mock recorder for MockHelper.
- `MockHelperMockRecorder.Apply` — Apply indicates an expected call of Apply.
- `MockHelperMockRecorder.ApplyRuntimeObject` — ApplyRuntimeObject indicates an expected call of ApplyRuntimeObject.
- `MockHelperMockRecorder.Create` — Create indicates an expected call of Create.
- `MockHelperMockRecorder.CreateOrUpdate` — CreateOrUpdate indicates an expected call of CreateOrUpdate.
- `MockHelperMockRecorder.CreateOrUpdateRuntimeObject` — CreateOrUpdateRuntimeObject indicates an expected call of CreateOrUpdateRuntimeObject.
- `MockHelperMockRecorder.CreateRuntimeObject` — CreateRuntimeObject indicates an expected call of CreateRuntimeObject.
- `MockHelperMockRecorder.Delete` — Delete indicates an expected call of Delete.
- `MockHelperMockRecorder.Info` — Info indicates an expected call of Info.
- `MockHelperMockRecorder.Patch` — Patch indicates an expected call of Patch.

## Internal Dependencies

- `github.com/openshift/hive/pkg/resource`
- `go.uber.org/mock/gomock`
- `k8s.io/apimachinery/pkg/runtime`
- `k8s.io/apimachinery/pkg/types`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (5 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/resource/mock`.

## Understanding Score

0.0

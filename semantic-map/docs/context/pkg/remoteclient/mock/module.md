<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/remoteclient/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockBuilder` — MockBuilder is a mock of Builder interface.
- `MockBuilder.Build` — Build mocks base method.
- `MockBuilder.BuildDynamic` — BuildDynamic mocks base method.
- `MockBuilder.BuildKubeClient` — BuildKubeClient mocks base method.
- `MockBuilder.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockBuilder.RESTConfig` — RESTConfig mocks base method.
- `MockBuilder.UsePrimaryAPIURL` — UsePrimaryAPIURL mocks base method.
- `MockBuilder.UseSecondaryAPIURL` — UseSecondaryAPIURL mocks base method.
- `MockBuilderMockRecorder` — MockBuilderMockRecorder is the mock recorder for MockBuilder.
- `MockBuilderMockRecorder.Build` — Build indicates an expected call of Build.
- `MockBuilderMockRecorder.BuildDynamic` — BuildDynamic indicates an expected call of BuildDynamic.
- `MockBuilderMockRecorder.BuildKubeClient` — BuildKubeClient indicates an expected call of BuildKubeClient.
- `MockBuilderMockRecorder.RESTConfig` — RESTConfig indicates an expected call of RESTConfig.
- `MockBuilderMockRecorder.UsePrimaryAPIURL` — UsePrimaryAPIURL indicates an expected call of UsePrimaryAPIURL.
- `MockBuilderMockRecorder.UseSecondaryAPIURL` — UseSecondaryAPIURL indicates an expected call of UseSecondaryAPIURL.

## Internal Dependencies

- `github.com/openshift/hive/pkg/remoteclient`
- `go.uber.org/mock/gomock`
- `k8s.io/client-go/dynamic`
- `k8s.io/client-go/kubernetes`
- `k8s.io/client-go/rest`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (7 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/remoteclient/mock`.

## Understanding Score

0.0

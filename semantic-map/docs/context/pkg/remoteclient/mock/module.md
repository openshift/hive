# Module: pkg/remoteclient/mock

## Responsibility

Generated gomock mock for the `remoteclient.Builder` interface. Provides `MockBuilder` for use in unit tests of controllers and other packages that depend on remote cluster client construction.

## Public Interface/API

- **`NewMockBuilder(ctrl *gomock.Controller) *MockBuilder`** -- Constructor.
- **`MockBuilder`** -- Mocks all `Builder` methods: `Build`, `BuildDynamic`, `BuildKubeClient`, `RESTConfig`, `UsePrimaryAPIURL`, `UseSecondaryAPIURL`.
- **`MockBuilderMockRecorder`** -- Expectation recorder with matching methods.

## Internal Dependencies

- `github.com/golang/mock/gomock`
- `github.com/openshift/hive/pkg/remoteclient`
- `k8s.io/client-go/{dynamic,kubernetes,rest}`
- `sigs.k8s.io/controller-runtime/pkg/client`

## Capabilities

- **Package name**: `mock`
- **Package ID**: `github.com/openshift/hive/pkg/remoteclient/mock`
- **Source file**: `remoteclient_generated.go` (auto-generated via `go:generate mockgen`)

## Understanding Score

0.95

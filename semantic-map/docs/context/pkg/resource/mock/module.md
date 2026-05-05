# Module: pkg/resource/mock

## Responsibility

Generated gomock mock for the `resource.Helper` interface. Provides `MockHelper` for use in unit tests of controllers and other packages that perform apply/create/patch/delete operations against clusters.

## Public Interface/API

- **`NewMockHelper(ctrl *gomock.Controller) *MockHelper`** -- Constructor.
- **`MockHelper`** -- Mocks all `Helper` methods: `Apply`, `ApplyRuntimeObject`, `Create`, `CreateRuntimeObject`, `CreateOrUpdate`, `CreateOrUpdateRuntimeObject`, `Info`, `Patch`, `Delete`.
- **`MockHelperMockRecorder`** -- Expectation recorder with matching methods for all 9 operations.

## Internal Dependencies

- `github.com/golang/mock/gomock`
- `github.com/openshift/hive/pkg/resource`
- `k8s.io/apimachinery/pkg/{runtime,types}`

## Capabilities

- **Package name**: `mock`
- **Package ID**: `github.com/openshift/hive/pkg/resource/mock`
- **Source file**: `helper_generated.go` (auto-generated via `go:generate mockgen`)

## Understanding Score

0.95

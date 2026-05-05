# Module atlas

## Responsibility

Generated gomock mock for `pkg/azureclient.Client` and its paginated result interfaces, used in unit tests.

## Public Interface/API

- `MockClient` — mock of `azureclient.Client` (DNS zones, record sets, VMs, SKUs, images)
- `MockRecordSetPage`, `MockResourceSKUsPage`, `MockImageListResultPage` — mock paginated results
- Corresponding `MockRecorder` types for expectation setup

## Internal Dependencies

- `github.com/golang/mock/gomock` — mock framework (external)
- `github.com/openshift/hive/pkg/azureclient` — source interfaces

## Capabilities

- Auto-generated via `mockgen -source=./client.go`
- Provides test doubles for all Azure API operations and paginated result types

## Understanding Score

0.95

# Module atlas

## Responsibility

Test utilities for creating fake controller-runtime clients. Provides two facilities: (1) `NewFakeClientBuilder` which creates a properly configured `fake.ClientBuilder` with the Hive scheme and all known types registered as status subresources, and (2) `FakeClientWithCustomErrors` which wraps a fake client to inject per-call errors on Get, List, Delete, and Update operations.

## Public Interface/API

- `NewFakeClientBuilder() *fake.ClientBuilder` -- creates a ClientBuilder with the Hive scheme (hivev1, hivecontracts, hiveinternal) and all known types registered as status subresources
- `FakeClientWithCustomErrors` (struct) -- wraps a `crclient.Client`, overriding Get/List/Delete/Update with configurable error sequences:
  - `GetBehavior []error` -- per-call error overrides for Get
  - `ListBehavior []error` -- per-call error overrides for List
  - `DeleteBehavior []error` -- per-call error overrides for Delete
  - `UpdateBehavior []error` -- per-call error overrides for Update
  - Methods: `Get(ctx, key, obj, opts...)`, `List(ctx, list, opts...)`, `Delete(ctx, obj, opts...)`, `Update(ctx, obj, opts...)`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`, `hivecontracts/v1alpha1`, `hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/util/scheme`
- `sigs.k8s.io/controller-runtime/pkg/client`, `sigs.k8s.io/controller-runtime/pkg/client/fake`
- `context`, `reflect`

## Capabilities

- **Package**: `fake`
- Not a builder package; provides test client infrastructure
- `NewFakeClientBuilder` uses reflection to discover all Hive types and register them as status subresources

## Understanding Score

0.9

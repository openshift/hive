# Module atlas

## Responsibility

Provides test utilities for creating fake controller-runtime clients: a pre-configured fake client builder with all Hive schemes registered, and a wrapper that allows injecting custom errors into specific client method calls.

## Public Interface/API

- `NewFakeClientBuilder() *fake.ClientBuilder` -- creates a fake.ClientBuilder with the full Hive scheme (hivev1, hivecontracts, hiveinternal) and all known types registered as status sub-resources
- `FakeClientWithCustomErrors` -- struct embedding `crclient.Client` that overrides Get, List, Delete, Update to conditionally return injected errors
  - Fields: `GetBehavior []error`, `ListBehavior []error`, `DeleteBehavior []error`, `UpdateBehavior []error`
  - `Get(ctx, key, obj, opts...) error` -- uses GetBehavior to conditionally bypass real Get
  - `List(ctx, list, opts...) error` -- uses ListBehavior to conditionally bypass real List
  - `Delete(ctx, obj, opts...) error` -- uses DeleteBehavior to conditionally bypass real Delete
  - `Update(ctx, obj, opts...) error` -- uses UpdateBehavior to conditionally bypass real Update

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/util/scheme`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/fake`

## Capabilities

- Creates fake controller-runtime clients with full Hive scheme registration and status sub-resource support
- Allows per-call error injection on Get, List, Delete, and Update operations via behavior arrays
- Each behavior array entry maps to a successive call; nil entries pass through to the real fake; non-nil entries short-circuit with the specified error

## Understanding Score

0.9

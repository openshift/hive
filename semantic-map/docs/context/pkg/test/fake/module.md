<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/test/fake/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `FakeClientWithCustomErrors` — FakeClientWithCustomErrors overrides some of the fake client's methods, allowing them to (not actually run and) throw specific errors. Use it like this:
- `FakeClientWithCustomErrors.Delete` — Delete overrides the fake client's Delete, conditionally bypassing it and returning an error instead.
- `FakeClientWithCustomErrors.Get` — Get overrides the fake client's Get, conditionally bypassing it and returning an error instead.
- `FakeClientWithCustomErrors.List` — List overrides the fake client's List, conditionally bypassing it and returning an error instead.
- `FakeClientWithCustomErrors.Update` — Update overrides the fake client's Update, conditionally bypassing it and returning an error instead.
- `NewFakeClientBuilder` — Wrapper around fake client which registers all necessary types as Status sub-resource and adds the hive scheme to the client.

## Internal Dependencies

- `context`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/apis/hivecontracts/v1alpha1`
- `github.com/openshift/hive/apis/hiveinternal/v1alpha1`
- `github.com/openshift/hive/pkg/util/scheme`
- `reflect`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `sigs.k8s.io/controller-runtime/pkg/client/fake`

## Capabilities

- **`package`** name(s): **fake**.
- Go **`import`** edges listed below (8 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/test/fake`.

## Understanding Score

0.0

<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/dnsendpoint/nameserver/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockQuery` — MockQuery is a mock of Query interface.
- `MockQuery.CreateOrUpdate` — CreateOrUpdate mocks base method.
- `MockQuery.Delete` — Delete mocks base method.
- `MockQuery.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockQuery.Get` — Get mocks base method.
- `MockQueryMockRecorder` — MockQueryMockRecorder is the mock recorder for MockQuery.
- `MockQueryMockRecorder.CreateOrUpdate` — CreateOrUpdate indicates an expected call of CreateOrUpdate.
- `MockQueryMockRecorder.Delete` — Delete indicates an expected call of Delete.
- `MockQueryMockRecorder.Get` — Get indicates an expected call of Get.

## Internal Dependencies

- `go.uber.org/mock/gomock`
- `k8s.io/apimachinery/pkg/util/sets`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (3 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/dnsendpoint/nameserver/mock`.

## Understanding Score

0.0

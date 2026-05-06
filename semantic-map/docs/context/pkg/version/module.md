<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/version/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Get` — Get returns the overall codebase version. It's for detecting what code a binary was built from.
- `String` — String returns a human-friendly version.

## Internal Dependencies

- `fmt`
- `k8s.io/apimachinery/pkg/version`

## Capabilities

- **`package`** name(s): **version**.
- Go **`import`** edges listed below (2 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/version`.

## Understanding Score

0.0

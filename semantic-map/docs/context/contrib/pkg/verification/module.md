<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/verification/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `NewVerifyImportsCommand` — NewVerifyImportsCommand adds a subcommand for verifying imports of a go file.
- `VerifyImportsOptions` — VerifyImportsOptions contains the options for verifying go imports
- `VerifyImportsOptions.VerifyImports` — VerifyImports verifies that the imports match the required convention.

## Internal Dependencies

- `fmt`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `go/ast`
- `go/parser`
- `go/token`
- `gopkg.in/yaml.v2`
- `k8s.io/apimachinery/pkg/util/errors`
- `os`

## Capabilities

- **`package`** name(s): **verification**.
- Go **`import`** edges listed below (9 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/verification`.

## Understanding Score

0.0

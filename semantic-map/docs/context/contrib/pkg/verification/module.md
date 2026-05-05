# Module atlas

## Responsibility

Implements the `verify-imports` command for hiveutil: parses Go source files using the Go AST and verifies that import statements conform to a YAML-configured grouping convention.

## Public Interface/API

- `NewVerifyImportsCommand() *cobra.Command` — `verify-imports` subcommand
- `VerifyImportsOptions` — exported options struct (import group config, file paths)
- `VerifyImportsOptions.VerifyImports() error` — parses each Go file's imports and checks group ordering/separation

## Internal Dependencies

- `go/ast`, `go/parser`, `go/token` — Go AST parsing for import analysis
- `gopkg.in/yaml.v2` — YAML config for import group rules
- `k8s.io/apimachinery/pkg/util/errors` — error aggregation

## Capabilities

- Parses Go source files and extracts import block structure
- Validates import group ordering and blank-line separation against a configurable YAML ruleset
- Reports violations with file and line information
- Used as a CI lint step for import hygiene

## Understanding Score

0.9

# Module atlas

## Responsibility

Provides a CLI command for the `hiveutil` tool to verify that Go source file imports follow naming conventions defined in a YAML configuration file.

## Public Interface/API

**Types:**
- `VerifyImportsOptions` -- Options containing the Go file path, config file path, and logger

**Functions:**
- `NewVerifyImportsCommand() *cobra.Command` -- Creates the `verify-imports` command that checks a Go file's imports against configured rules
- `(o *VerifyImportsOptions) VerifyImports() error` -- Parses the Go file, loads import naming rules from YAML config, and validates import aliases match the rules

## Internal Dependencies

None (this package has no Hive-internal dependencies; it only uses Go standard library, `gopkg.in/yaml.v2`, `k8s.io/apimachinery/pkg/util/errors`, and `github.com/spf13/cobra`)

## Capabilities

- Parse Go source files to extract import statements
- Load import naming convention rules from a YAML config file
- Validate that import aliases match required naming conventions
- Report all violations as aggregated errors

## Understanding Score

0.9

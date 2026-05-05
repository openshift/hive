# Module: pkg/manageddns

## Responsibility

Reads the Hive managed DNS domains configuration from a JSON file on disk. The file path is specified by the `MANAGED_DOMAINS_FILE` environment variable (via `constants.ManagedDomainsFileEnvVar`). This is a small utility package used by controllers that need to know which DNS domains Hive is configured to manage.

## Public Interface/API

- `ReadManagedDomainsFile() ([]hivev1.ManageDNSConfig, error)` -- reads the `MANAGED_DOMAINS_FILE` env var, loads and unmarshals the JSON file into a slice of `hivev1.ManageDNSConfig`. Returns `(nil, nil)` if the env var is not set (i.e., managed domains are not configured).

## Internal Dependencies

- `apis/hive/v1` -- `ManageDNSConfig` type
- `pkg/constants` -- `ManagedDomainsFileEnvVar` constant

No external dependencies beyond Go stdlib (`encoding/json`, `os`).

## Capabilities

- **Package**: `manageddns`
- Single source file: `manageddns.go` (31 lines).
- 4 unique import paths.

## Understanding Score

0.92

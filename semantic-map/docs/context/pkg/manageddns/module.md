# Module atlas

## Responsibility

Reads the Hive managed DNS domains configuration from a JSON file specified by environment variable.

## Public Interface/API

- `ReadManagedDomainsFile() ([]hivev1.ManageDNSConfig, error)` -- reads the file pointed to by `ManagedDomainsFileEnvVar` and unmarshals it into a slice of `ManageDNSConfig`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/pkg/constants`

## Capabilities

- Loads managed DNS domain configurations from a JSON file at a path specified by environment variable
- Returns nil (not error) when the env var is unset

## Understanding Score

0.9

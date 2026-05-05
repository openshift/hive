# Module atlas

## Responsibility

Manages the concept of "contract implementations" -- a mechanism for declaring which Kubernetes resource types (identified by GVK) implement a named contract. This allows Hive to support pluggable implementations of contracts (e.g. the `clusterinstall` contract) by loading a JSON configuration file at runtime from a path specified via the `SUPPORTED_CONTRACT_IMPLEMENTATIONS_FILE` environment variable. The package provides lookup, membership-check, and per-implementation config retrieval for contract implementations.

## Public Interface/API

- `ContractImplementation` -- struct representing a resource (Group, Version, Kind, optional Config map) that implements a contract.
- `SupportedContractImplementations` -- struct binding a contract name to a list of `ContractImplementation` entries.
- `SupportedContractImplementationsList` -- `[]SupportedContractImplementations`, with the following methods:
  - `SupportedImplementations(contract string) []string` -- returns stringified GVKs for all implementations of a named contract.
  - `IsSupported(contract string, impl ContractImplementation) bool` -- checks whether a specific GVK is a supported implementation of a contract.
  - `GetConfig(contract string, impl ContractImplementation) map[string]string` -- returns the config map for a specific implementation under a contract, or an empty map if not found.
- `ReadSupportContractsFile() (SupportedContractImplementationsList, error)` -- reads and unmarshals the JSON config file whose path comes from `constants.SupportedContractImplementationsFileEnvVar`. Returns nil if the env var is unset or the file does not exist.

## Internal Dependencies

- `encoding/json` -- JSON unmarshalling of the config file.
- `os` -- environment variable and file reading.
- `github.com/openshift/hive/pkg/constants` -- provides `SupportedContractImplementationsFileEnvVar`.
- `k8s.io/apimachinery/pkg/runtime/schema` -- `GroupVersionKind` for stringifying implementations.

## Capabilities

- **`package`** name(s): **contracts**.
- Go **`import`** edges listed below (4 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/contracts`.
- Test file (`contracts_test.go`) covers `SupportedImplementations` and `IsSupported` with multi-contract fixtures.

## Understanding Score

0.85

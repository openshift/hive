# Module atlas

## Responsibility

Defines data structures and lookup logic for Hive contract implementations -- resources that fulfill a named contract (e.g. a specific GVK). Reads supported contract implementations from a JSON configuration file specified by an environment variable.

## Public Interface/API

- `type ContractImplementation struct` -- a GVK plus optional config map representing a contract-implementing resource
- `type SupportedContractImplementations struct` -- a named contract with its list of supported implementations
- `type SupportedContractImplementationsList []SupportedContractImplementations` -- list of all contracts
- `(SupportedContractImplementationsList).SupportedImplementations(contract string) []string` -- returns GVK strings for a contract
- `(SupportedContractImplementationsList).IsSupported(contract string, impl ContractImplementation) bool` -- checks if a GVK is supported for a contract
- `(SupportedContractImplementationsList).GetConfig(contract string, impl ContractImplementation) map[string]string` -- returns config map for a supported implementation
- `ReadSupportContractsFile() (SupportedContractImplementationsList, error)` -- reads and parses the contracts JSON file from the path in env var

## Internal Dependencies

- `github.com/openshift/hive/pkg/constants` -- SupportedContractImplementationsFileEnvVar
- `k8s.io/apimachinery/pkg/runtime/schema` -- GroupVersionKind
- `encoding/json`, `os` -- file reading and JSON parsing

## Capabilities

- Load contract implementation definitions from a JSON configuration file
- Query whether a GVK is a supported implementation for a named contract
- Retrieve config metadata for a contract implementation
- List all supported GVK strings for a contract

## Understanding Score

0.85

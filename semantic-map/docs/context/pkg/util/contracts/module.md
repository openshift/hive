<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/util/contracts/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ContractImplementation` — ContractImplementation is a resources that implements some contract
- `SupportedContractImplementations` — SupportedContractImplementations defines a list of resources that implement a contract
- `SupportedContractImplementationsList` — SupportedContractImplementationsList is a list of contracts and their supported implementations
- `SupportedContractImplementationsList.GetConfig`
- `SupportedContractImplementationsList.IsSupported`
- `SupportedContractImplementationsList.SupportedImplementations`

## Internal Dependencies

- `encoding/json`
- `github.com/openshift/hive/pkg/constants`
- `k8s.io/apimachinery/pkg/runtime/schema`
- `os`

## Capabilities

- **`package`** name(s): **contracts**.
- Go **`import`** edges listed below (4 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/util/contracts`.

## Understanding Score

0.0

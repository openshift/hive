<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/azureclient/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `Client` — Client is a wrapper object for actual Azure libraries to allow for easier mocking/testing.
- `ImageListResultPage`
- `RecordSetPage` — RecordSetPage is a page of results from listing record sets.
- `ResourceSKUsPage` — ResourceSKUsPage is a page of results from listing resource SKUs.

## Internal Dependencies

- `context`
- `encoding/json`
- `fmt`
- `github.com/Azure/azure-sdk-for-go/profiles/latest/compute/mgmt/compute`
- `github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-12-01/compute`
- `github.com/Azure/azure-sdk-for-go/services/dns/mgmt/2018-05-01/dns`
- `github.com/Azure/go-autorest/autorest`
- `github.com/Azure/go-autorest/autorest/azure`
- `github.com/Azure/go-autorest/autorest/azure/auth`
- `github.com/Azure/go-autorest/autorest/to`
- `github.com/openshift/hive/pkg/constants`
- `github.com/openshift/installer/pkg/asset/installconfig/azure`
- `github.com/pkg/errors`
- `k8s.io/api/core/v1`
- `os`

## Capabilities

- **`package`** name(s): **azureclient**.
- Go **`import`** edges listed below (15 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/azureclient`.

## Understanding Score

0.0

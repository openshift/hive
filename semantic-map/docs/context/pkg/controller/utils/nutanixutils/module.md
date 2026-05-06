<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/controller/utils/nutanixutils/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `ConvertHiveFailureDomains` — ConvertHiveFailureDomains converts Hive failure domains to Installer failure domains and returns unique PrismElements and SubnetUUIDs.
- `ConvertInstallerFailureDomains` — ConvertInstallerFailureDomains converts Installer failure domains to Hive failure domains and returns unique PrismElements and SubnetUUIDs.
- `ExtractInstallerResources` — ExtractInstallerResources extracts unique PrismElements and Subnet UUIDs from a slice of Installer failure domains. This function iterates through the provided failure domains, co…

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1/nutanix`
- `github.com/openshift/installer/pkg/types/nutanix`
- `github.com/pkg/errors`
- `k8s.io/apimachinery/pkg/util/sets`

## Capabilities

- **`package`** name(s): **nutanixutils**.
- Go **`import`** edges listed below (4 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/controller/utils/nutanixutils`.

## Understanding Score

0.0

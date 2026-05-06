<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`contrib/pkg/report/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `DeprovisioningReportOptions` — DeprovisioningReportOptions is the set of options for the desired report.
- `DeprovisioningReportOptions.Complete` — Complete finishes parsing arguments for the command
- `DeprovisioningReportOptions.Run` — Run executes the command
- `DeprovisioningReportOptions.Validate` — Validate ensures that option values make sense
- `NewClusterReportCommand` — NewClusterReportCommand creates a command that generates and outputs the cluster report.
- `NewDeprovisioningReportCommand` — NewDeprovisioningReportCommand creates a command that generates and outputs the cluster report.
- `NewProvisioningReportCommand` — NewProvisioningReportCommand creates a command that generates and outputs the cluster report.
- `ProvisioningReportOptions` — ProvisioningReportOptions is the set of options for the desired report.
- `ProvisioningReportOptions.Complete` — Complete finishes parsing arguments for the command
- `ProvisioningReportOptions.Run` — Run executes the command
- `ProvisioningReportOptions.Validate` — Validate ensures that option values make sense

## Internal Dependencies

- `context`
- `fmt`
- `github.com/openshift/hive/apis/hive/v1`
- `github.com/openshift/hive/contrib/pkg/utils`
- `github.com/sirupsen/logrus`
- `github.com/spf13/cobra`
- `k8s.io/api/core/v1`
- `k8s.io/apimachinery/pkg/api/errors`
- `k8s.io/apimachinery/pkg/types`
- `sigs.k8s.io/controller-runtime/pkg/client`
- `time`

## Capabilities

- **`package`** name(s): **report**.
- Go **`import`** edges listed below (11 unique path(s)).
- Package ID(s): `github.com/openshift/hive/contrib/pkg/report`.

## Understanding Score

0.0

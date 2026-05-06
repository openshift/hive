# Module atlas

## Responsibility

Entry-point binary for `hiveutil`, a multi-subcommand CLI utility for running, testing, and administering Hive clusters. Aggregates subcommands from various `contrib/pkg` packages and core Hive packages.

## Public Interface/API

`main` package — no exported identifiers. Produces the `hiveutil` binary.

- `main()` — builds the root cobra command and executes it
- `newHiveutilCommand()` — unexported; creates the root command and registers all subcommands

Registered subcommands:
- `deprovision.NewDeprovisionAWSWithTagsCommand()` — deprovision AWS resources by tags
- `deprovision.NewDeprovisionCommand()` — deprovision cluster resources
- `verification.NewVerifyImportsCommand()` — verify import consistency
- `installmanager.NewInstallManagerCommand()` — install manager CLI
- `imageset.NewUpdateInstallerImageCommand()` — update installer image
- `testresource.NewTestResourceCommand()` — create test resources
- `createcluster.NewCreateClusterCommand()` — create a cluster
- `report.NewClusterReportCommand()` — cluster reporting
- `certificate.NewCertificateCommand()` — certificate management
- `adm.NewAdmCommand()` — admin utilities
- `version.NewVersionCommand()` — print version
- `clusterpool.NewClusterPoolCommand()` — cluster pool management
- `awsprivatelink.NewAWSPrivateLinkCommand()` — AWS PrivateLink management

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/adm` — admin subcommand
- `github.com/openshift/hive/contrib/pkg/awsprivatelink` — AWS PrivateLink subcommand
- `github.com/openshift/hive/contrib/pkg/certificate` — certificate subcommand
- `github.com/openshift/hive/contrib/pkg/clusterpool` — cluster pool subcommand
- `github.com/openshift/hive/contrib/pkg/createcluster` — create cluster subcommand
- `github.com/openshift/hive/contrib/pkg/deprovision` — deprovision subcommands
- `github.com/openshift/hive/contrib/pkg/report` — cluster report subcommand
- `github.com/openshift/hive/contrib/pkg/testresource` — test resource subcommand
- `github.com/openshift/hive/contrib/pkg/verification` — import verification subcommand
- `github.com/openshift/hive/contrib/pkg/version` — version subcommand
- `github.com/openshift/hive/pkg/imageset` — installer image update command
- `github.com/openshift/hive/pkg/installmanager` — install manager command
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging

## Capabilities

- Provides a unified CLI for Hive administration and testing tasks
- Aggregates 13 subcommands spanning cluster lifecycle (create, deprovision), resource management (certificates, cluster pools, AWS PrivateLink), reporting, and install management
- Acts as the primary developer/operator utility tool for Hive

## Understanding Score

0.9

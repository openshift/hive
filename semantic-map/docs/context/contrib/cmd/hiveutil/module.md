# Module atlas

## Responsibility

Binary entry point for the `hiveutil` CLI. Assembles all contrib subcommands into a single cobra command tree for cluster creation, deprovision, pool management, reporting, and operational utilities.

## Public Interface/API

This is a `main` package — no exported identifiers. The binary provides:

- `hiveutil` CLI with subcommands: `create-cluster`, `deprovision`, `deprovision-aws-tags`, `verify-imports`, `install-manager`, `update-installer-image`, `resource`, `report`, `certificate`, `adm`, `version`, `clusterpool`, `awsprivatelink`

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/adm` — `adm` subcommand
- `github.com/openshift/hive/contrib/pkg/awsprivatelink` — `awsprivatelink` subcommand
- `github.com/openshift/hive/contrib/pkg/certificate` — `certificate` subcommand
- `github.com/openshift/hive/contrib/pkg/clusterpool` — `clusterpool` subcommand
- `github.com/openshift/hive/contrib/pkg/createcluster` — `create-cluster` subcommand
- `github.com/openshift/hive/contrib/pkg/deprovision` — `deprovision` subcommand
- `github.com/openshift/hive/contrib/pkg/report` — `report` subcommand
- `github.com/openshift/hive/contrib/pkg/testresource` — `resource` subcommand
- `github.com/openshift/hive/contrib/pkg/verification` — `verify-imports` subcommand
- `github.com/openshift/hive/contrib/pkg/version` — `version` subcommand
- `github.com/openshift/hive/pkg/imageset` — `update-installer-image` subcommand
- `github.com/openshift/hive/pkg/installmanager` — `install-manager` subcommand

## Capabilities

- Provides a single CLI binary for Hive operational tasks outside the controller loop
- Assembles 13 subcommands from contrib/pkg and core pkg packages
- Each subcommand is independently implemented in its own package

## Understanding Score

0.85

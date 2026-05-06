# Module atlas

## Responsibility

Provides the `adm` cobra subcommand grouping for Hive administration utilities. Currently contains only the `manage-dns` subcommand.

## Public Interface/API

- `func NewAdmCommand() *cobra.Command` — creates the `adm` subcommand and registers child commands (currently `managedns.NewManageDNSCommand()`)

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/adm/managedns` — managed DNS subcommand
- `github.com/spf13/cobra` — CLI framework

## Capabilities

- Groups Hive administration subcommands under `hiveutil adm`
- Delegates to `managedns` for DNS management functionality

## Understanding Score

0.9

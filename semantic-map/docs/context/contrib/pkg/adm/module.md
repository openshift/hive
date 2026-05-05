# Module atlas

## Responsibility

Provides the `adm` subcommand group for hiveutil, aggregating Hive administration commands.

## Public Interface/API

- `NewAdmCommand() *cobra.Command` — returns the `adm` cobra command with `manage-dns` as a subcommand

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/adm/managedns` — `manage-dns` subcommand

## Capabilities

- Acts as a command group container for Hive administration subcommands
- Currently delegates solely to `managedns.NewManageDNSCommand()`

## Understanding Score

0.9

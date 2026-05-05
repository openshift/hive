# Module atlas

## Responsibility

Implements the `manage-dns enable` subcommand for hiveutil: configures managed DNS globally in HiveConfig by creating cloud credential Secrets and updating HiveConfig.spec.managedDomains.

## Public Interface/API

- `NewManageDNSCommand() *cobra.Command` — top-level `manage-dns` command
- `NewEnableManageDNSCommand() *cobra.Command` — `enable` subcommand
- `Options` — exported options struct
- `Options.Complete(cmd, args) error` — finalizes options
- `Options.Validate(cmd) error` — validates options
- `Options.Run() error` — executes the command

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — HiveConfig, ManagedDomain types
- `github.com/openshift/hive/contrib/pkg/utils` — GetClient, GetResourceHelper
- `github.com/openshift/hive/pkg/constants` — platform constants
- `github.com/openshift/hive/pkg/creds/{aws,azure,gcp}` — cloud credential loading
- `github.com/openshift/hive/pkg/resource` — resource helper for apply operations
- `github.com/openshift/hive/pkg/util/scheme` — shared scheme
- `sigs.k8s.io/controller-runtime/pkg/client` — dynamic client (external)

## Capabilities

- Enables managed DNS for AWS, Azure, or GCP by creating cloud credential Secrets in Hive's namespace
- Updates HiveConfig.spec.managedDomains with the configured domains
- Watches for HiveConfig controller deployment rollout after configuration changes

## Understanding Score

0.75

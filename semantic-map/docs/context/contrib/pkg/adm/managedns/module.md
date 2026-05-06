# Module atlas

## Responsibility

Implements the `hiveutil adm manage-dns enable` subcommand, which configures global managed DNS in HiveConfig by creating cloud credentials secrets and updating the HiveConfig CRD for AWS, GCP, or Azure.

## Public Interface/API

**Types:**
- `type Options struct` — holds cloud provider, credentials file path, Azure resource group, and hive client

**Functions:**
- `func NewManageDNSCommand() *cobra.Command` — creates the `manage-dns` parent subcommand
- `func NewEnableManageDNSCommand() *cobra.Command` — creates the `enable` subcommand with flags `--cloud`, `--creds-file`, `--azure-resource-group-name`

**Methods on Options:**
- `func (o *Options) Complete(cmd *cobra.Command, args []string) error` — resolves home directory
- `func (o *Options) Validate(cmd *cobra.Command) error` — validates options (currently no-op)
- `func (o *Options) Run(args []string) error` — executes the enable workflow: creates credentials secret, updates HiveConfig, waits for rollout

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — HiveConfig, ManageDNSConfig types
- `github.com/openshift/hive/contrib/pkg/utils` — `GetClient` helper
- `github.com/openshift/hive/pkg/constants` — platform constants, credential key names
- `github.com/openshift/hive/pkg/creds/aws` — `GetAWSCreds`
- `github.com/openshift/hive/pkg/creds/azure` — `GetCreds`
- `github.com/openshift/hive/pkg/creds/gcp` — `GetCreds`
- `github.com/openshift/hive/pkg/resource` — `Helper` for applying runtime objects
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `github.com/google/uuid` — unique secret name suffixes
- `sigs.k8s.io/controller-runtime/pkg/client` — Kubernetes client with watch
- `k8s.io/kubectl/pkg/polymorphichelpers` — deployment status viewer

## Capabilities

- Creates cloud-provider-specific credentials secrets (AWS, GCP, Azure) from local credential files
- Updates HiveConfig to add managed DNS domains with the corresponding credentials reference
- Waits for HiveConfig to be processed (ObservedGeneration matches Generation, ConfigApplied is true)
- Waits for hiveadmission deployment rollout to complete after HiveConfig update
- Supports `--cloud` flag for AWS (default), GCP, and Azure

## Understanding Score

0.9

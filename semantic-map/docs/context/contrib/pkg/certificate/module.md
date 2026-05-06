# Module atlas

## Responsibility

Implements the `hiveutil certificate` subcommand tree for creating Let's Encrypt TLS certificates for OpenShift clusters on AWS, using Route53 DNS-01 challenge validation via certbot hooks.

## Public Interface/API

**Types:**
- `type Options struct` — options for certificate creation (Name, BaseDomain, Email, Region, OutputDir, WaitTime)
- `type HookOptions struct` — options for DNS auth/cleanup hooks (HostedZoneID, Domain, Value, Region, WaitTime)

**Functions:**
- `func NewCertificateCommand() *cobra.Command` — parent `certificate` command; registers `create`, `auth-hook`, `cleanup-hook` subcommands
- `func NewCreateCertifcateCommand() *cobra.Command` — `create CLUSTER_NAME`; invokes certbot with DNS-01 challenge using auth/cleanup hooks. Flags: `--base-domain`, `--email`, `--region`, `--output-dir`, `--wait-time`
- `func NewAuthHookCommand() *cobra.Command` — `auth-hook REGION HOSTEDZONEID`; creates `_acme-challenge` TXT record in Route53
- `func NewCleanupHookCommand() *cobra.Command` — `cleanup-hook REGION HOSTEDZONEID`; deletes the TXT record

**Methods on Options:**
- `func (o *Options) Complete(cmd *cobra.Command, args []string) error`
- `func (o *Options) Run() error` — runs certbot, copies resulting cert/key/CA files to output directory

**Methods on HookOptions:**
- `func (o *HookOptions) Complete(cmd *cobra.Command, args []string) error` — reads `CERTBOT_DOMAIN` and `CERTBOT_VALIDATION` env vars
- `func (o *HookOptions) Create() error` — creates DNS TXT record, waits for propagation
- `func (o *HookOptions) Delete() error` — deletes DNS TXT record

## Internal Dependencies

- `github.com/openshift/hive/pkg/awsclient` — AWS client factory for Route53 operations
- `github.com/aws/aws-sdk-go-v2/service/route53` — Route53 API for hosted zone and record management
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `github.com/pkg/errors` — error wrapping

## Capabilities

- Creates Let's Encrypt serving certificates for `api.<name>.<domain>` and `*.apps.<name>.<domain>`
- Looks up the public Route53 hosted zone for the base domain
- Uses certbot with `--manual` DNS-01 challenge, delegating to self-invoked `auth-hook` and `cleanup-hook` subcommands
- Creates/deletes `_acme-challenge` TXT records in Route53 for ACME validation
- Outputs certificate, key, and CA chain files to the specified directory

## Understanding Score

0.9

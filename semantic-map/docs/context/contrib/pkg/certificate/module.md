# Module atlas

## Responsibility

Provides the `certificate` subcommand group for hiveutil: Let's Encrypt certificate creation and DNS-01 auth hook commands for AWS-hosted clusters.

## Public Interface/API

- `NewCertificateCommand() *cobra.Command` — top-level `certificate` command
- `NewCreateCertifcateCommand() *cobra.Command` — creates a Let's Encrypt certificate
- `NewAuthHookCommand() *cobra.Command` — DNS-01 challenge auth hook
- `NewCleanupHookCommand() *cobra.Command` — DNS-01 challenge cleanup hook
- `Options` — exported options struct for certificate creation
- `Options.Complete() error`, `Options.Run() error`
- `HookOptions` — exported options struct for DNS hooks
- `HookOptions.Complete() error`, `HookOptions.Create() error`, `HookOptions.Delete() error`

## Internal Dependencies

- `github.com/openshift/hive/pkg/awsclient` — Route53 API client for DNS record management
- `github.com/aws/aws-sdk-go-v2/service/route53` — Route53 service calls (external)

## Capabilities

- Creates TLS serving certificates for cluster control plane and routes using Let's Encrypt
- Provides DNS-01 challenge auth and cleanup hooks for automated certificate validation via Route53
- Targets AWS-hosted OpenShift clusters

## Understanding Score

0.75

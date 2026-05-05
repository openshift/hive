# Module atlas

## Responsibility

Provides the `awsprivatelink` subcommand group for hiveutil. Enables and disables AWS PrivateLink configuration in HiveConfig and manages endpoint VPC networking.

## Public Interface/API

- `NewAWSPrivateLinkCommand() *cobra.Command` — top-level command with subcommands: `enable`, `disable`, `endpointvpc`
- `NewEnableAWSPrivateLinkCommand() *cobra.Command` — configures HiveConfig with AWS PrivateLink, creates hub account credentials Secret, adds active cluster's VPC
- `NewDisableAWSPrivateLinkCommand() *cobra.Command` — removes AWS PrivateLink config from HiveConfig and deletes credentials Secrets
- `NewEndpointVPCCommand() *cobra.Command` — delegates to `endpointvpc` subpackage for add/remove operations
- Persistent flags: `--debug`, `--creds-secret` (namespace/name reference)

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/awsprivatelink/common` — shared state (DynamicClient, CredsSecret)
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/endpointvpc` — endpoint VPC add/remove commands
- `github.com/openshift/hive/contrib/pkg/utils` — `GetClient` for controller-runtime client
- `github.com/openshift/hive/pkg/awsclient` — AWS API client
- `github.com/openshift/hive/pkg/creds/aws` — AWS credential loading
- `github.com/openshift/hive/pkg/operator/hive` — `GetHiveNamespace` and `HiveOperatorNamespaceEnvVar`
- `github.com/openshift/hive/pkg/util/scheme` — shared scheme registration (used for configv1 types)
- `github.com/openshift/hive/apis/hive/v1` — HiveConfig and PrivateLink types
- `github.com/openshift/api/config/v1` — Infrastructure type for region/infraID lookup (external)
- `github.com/aws/aws-sdk-go-v2/service/ec2` — EC2 API calls for VPC discovery (external)

## Capabilities

- Enables AWS PrivateLink: detects cluster region/infraID from Infrastructure CR, finds VPC, creates hub credentials Secret, updates HiveConfig
- Disables AWS PrivateLink: removes credentials Secrets and clears HiveConfig.spec.awsPrivateLink
- Manages endpoint VPCs via subcommand delegation
- Supports credential source from env vars or an existing Secret on the cluster

## Understanding Score

0.85

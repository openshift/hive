# Module atlas

## Responsibility

Implements the `hiveutil awsprivatelink` subcommand tree for enabling, disabling, and managing AWS PrivateLink configuration in HiveConfig. Handles credentials secret creation, VPC discovery, and HiveConfig updates.

## Public Interface/API

**Functions:**
- `func NewAWSPrivateLinkCommand() *cobra.Command` — root `awsprivatelink` command with `--debug` and `--creds-secret` persistent flags; registers `enable`, `disable`, and `endpointvpc` subcommands
- `func NewEnableAWSPrivateLinkCommand() *cobra.Command` — `enable` subcommand; discovers the active cluster's VPC, creates hub account credentials secret, and configures `HiveConfig.spec.awsPrivateLink`. Flag: `--dns-record-type` (Alias|ARecord)
- `func NewDisableAWSPrivateLinkCommand() *cobra.Command` — `disable` subcommand; removes hub credentials secrets and clears `HiveConfig.spec.awsPrivateLink`
- `func NewEndpointVPCCommand() *cobra.Command` — `endpointvpc` parent command; delegates to `endpointvpc.NewEndpointVPCAddCommand()` and `endpointvpc.NewEndpointVPCRemoveCommand()`

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — HiveConfig, AWSPrivateLinkConfig types
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/common` — shared `DynamicClient` and `CredsSecret` vars
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/endpointvpc` — endpoint VPC add/remove subcommands
- `github.com/openshift/hive/contrib/pkg/utils` — `GetClient` helper
- `github.com/openshift/hive/pkg/awsclient` — AWS client factory (`NewClientFromSecret`)
- `github.com/openshift/hive/pkg/creds/aws` — `GetAWSCreds` for local credential extraction
- `github.com/openshift/hive/pkg/operator/hive` — `GetHiveNamespace`
- `github.com/openshift/hive/pkg/util/scheme` — aggregated CRD scheme
- `github.com/openshift/api/config/v1` — OpenShift Infrastructure type for region/infraID discovery
- `github.com/aws/aws-sdk-go-v2` — EC2 client for VPC discovery
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `sigs.k8s.io/controller-runtime/pkg/client` — Kubernetes dynamic client

## Capabilities

- Enables AWS PrivateLink by discovering the active cluster's VPC (via infra-id tag), creating a hub account credentials secret, and updating HiveConfig
- Disables AWS PrivateLink by cleaning up credentials secrets and clearing HiveConfig's awsPrivateLink section
- Supports using a pre-existing credentials secret on the cluster (`--creds-secret ns/name`) or extracting credentials from the local AWS environment
- Delegates endpoint VPC management to the `endpointvpc` sub-package
- Validates that HiveConfig has no remaining associated/endpoint VPCs before disabling

## Understanding Score

0.9

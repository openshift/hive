# Module atlas

## Responsibility

Implements the `hiveutil awsprivatelink endpointvpc add|remove` subcommands that set up and tear down AWS networking resources (VPC peering, routes, security groups) between endpoint VPCs and associated VPCs, and update HiveConfig accordingly.

## Public Interface/API

**Functions:**
- `func NewEndpointVPCAddCommand() *cobra.Command` — `add vpc-id` subcommand; required flags: `--region`, `--subnet-ids`. Sets up VPC peering, route tables, and security group rules, then adds the endpoint VPC to HiveConfig
- `func NewEndpointVPCRemoveCommand() *cobra.Command` — `remove vpc-id` subcommand. Tears down VPC peering, routes, and security group rules, then removes the endpoint VPC from HiveConfig

## Internal Dependencies

- `github.com/openshift/hive/apis/hive/v1` — HiveConfig, AWSPrivateLinkInventory, AWSAssociatedVPC types
- `github.com/openshift/hive/contrib/pkg/awsprivatelink/common` — shared `DynamicClient` and `CredsSecret`
- `github.com/openshift/hive/pkg/awsclient` — AWS client interface and factory (`NewClientFromSecret`, error code helpers)
- `github.com/aws/aws-sdk-go-v2` — EC2 service client for VPC, subnet, route table, security group, and peering operations
- `github.com/spf13/cobra` — CLI framework
- `github.com/sirupsen/logrus` — structured logging
- `k8s.io/apimachinery/pkg/util/sets` — region set for multi-region client creation

## Capabilities

- **Add:** Creates VPC peering connections between endpoint and associated VPCs, adds routes to private route tables, authorizes security group ingress (by SG reference for same-region, by CIDR for cross-region), and adds endpoint VPC to `HiveConfig.spec.awsPrivateLink.endpointVPCInventory`
- **Remove:** Deletes VPC peering connections, removes routes from route tables, revokes security group ingress rules, and removes endpoint VPC from HiveConfig
- Handles idempotency gracefully (tolerates already-existing routes/rules and not-found conditions)
- Helper functions for VPC/SG/CIDR lookups: `getDefaultSGOfVpc`, `getWorkerSGFromVpcId`, `getCIDRFromVpcId`, `getInfraIdFromVpcId`, `findVpcInInventory`, `getAWSClientsByRegion`

## Understanding Score

0.9

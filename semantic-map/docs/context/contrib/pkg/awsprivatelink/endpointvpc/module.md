# Module atlas

## Responsibility

Implements `endpointvpc add` and `endpointvpc remove` subcommands for managing AWS endpoint VPC networking — VPC peering, route tables, security groups — and updating HiveConfig's endpoint VPC inventory.

## Public Interface/API

- `NewEndpointVPCAddCommand() *cobra.Command` — `add vpc-id --region REGION --subnet-ids IDS`: peers endpoint VPC with associated VPCs, configures routes and security groups, adds to HiveConfig
- `NewEndpointVPCRemoveCommand() *cobra.Command` — removes an endpoint VPC and tears down networking

## Internal Dependencies

- `github.com/openshift/hive/contrib/pkg/awsprivatelink/common` — shared DynamicClient and CredsSecret
- `github.com/openshift/hive/pkg/awsclient` — AWS EC2 API client interface
- `github.com/openshift/hive/apis/hive/v1` — HiveConfig, AWSPrivateLinkInventory, AWSPrivateLinkSubnet types
- `github.com/aws/aws-sdk-go-v2` — EC2 service calls (external)

## Capabilities

- Creates VPC peering connections between endpoint and associated VPCs (including cross-region)
- Adds routes to relevant route tables in both VPCs
- Authorizes security group ingress (by SG for same-region, by CIDR for cross-region)
- Updates HiveConfig.spec.awsPrivateLink.endpointVPCInventory with subnet/AZ details
- Handles idempotent operations (tolerates already-existing routes, peering, SG rules)

## Understanding Score

0.8

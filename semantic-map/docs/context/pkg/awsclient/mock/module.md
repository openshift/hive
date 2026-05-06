<!-- semantic-map module stub v3 -->

# Module atlas

## Responsibility

One or more Go packages rooted at **`pkg/awsclient/mock/**` relative to this repository. Part of module **`github.com/openshift/hive`**.

## Public Interface/API

Deterministic exports from **`go/doc`** over **`go/packages`** syntax (one-line doc synopsis where available):

- `MockClient` — MockClient is a mock of Client interface.
- `MockClient.AcceptVpcPeeringConnection` — AcceptVpcPeeringConnection mocks base method.
- `MockClient.AssociateVPCWithHostedZone` — AssociateVPCWithHostedZone mocks base method.
- `MockClient.AuthorizeSecurityGroupIngress` — AuthorizeSecurityGroupIngress mocks base method.
- `MockClient.ChangeResourceRecordSets` — ChangeResourceRecordSets mocks base method.
- `MockClient.ChangeTagsForResource` — ChangeTagsForResource mocks base method.
- `MockClient.CreateHostedZone` — CreateHostedZone mocks base method.
- `MockClient.CreateRoute` — CreateRoute mocks base method.
- `MockClient.CreateVPCAssociationAuthorization` — CreateVPCAssociationAuthorization mocks base method.
- `MockClient.CreateVpcEndpoint` — CreateVpcEndpoint mocks base method.
- `MockClient.CreateVpcEndpointServiceConfiguration` — CreateVpcEndpointServiceConfiguration mocks base method.
- `MockClient.CreateVpcPeeringConnection` — CreateVpcPeeringConnection mocks base method.
- `MockClient.DeleteHostedZone` — DeleteHostedZone mocks base method.
- `MockClient.DeleteRoute` — DeleteRoute mocks base method.
- `MockClient.DeleteVPCAssociationAuthorization` — DeleteVPCAssociationAuthorization mocks base method.
- `MockClient.DeleteVpcEndpointServiceConfigurations` — DeleteVpcEndpointServiceConfigurations mocks base method.
- `MockClient.DeleteVpcEndpoints` — DeleteVpcEndpoints mocks base method.
- `MockClient.DeleteVpcPeeringConnection` — DeleteVpcPeeringConnection mocks base method.
- `MockClient.DescribeAvailabilityZones` — DescribeAvailabilityZones mocks base method.
- `MockClient.DescribeInstances` — DescribeInstances mocks base method.
- `MockClient.DescribeLoadBalancers` — DescribeLoadBalancers mocks base method.
- `MockClient.DescribeNetworkInterfaces` — DescribeNetworkInterfaces mocks base method.
- `MockClient.DescribeRouteTables` — DescribeRouteTables mocks base method.
- `MockClient.DescribeRouteTablesPages` — DescribeRouteTablesPages mocks base method.
- `MockClient.DescribeSecurityGroups` — DescribeSecurityGroups mocks base method.
- `MockClient.DescribeSubnets` — DescribeSubnets mocks base method.
- `MockClient.DescribeSubnetsPages` — DescribeSubnetsPages mocks base method.
- `MockClient.DescribeVpcEndpointServiceConfigurations` — DescribeVpcEndpointServiceConfigurations mocks base method.
- `MockClient.DescribeVpcEndpointServicePermissions` — DescribeVpcEndpointServicePermissions mocks base method.
- `MockClient.DescribeVpcEndpointServices` — DescribeVpcEndpointServices mocks base method.
- `MockClient.DescribeVpcEndpoints` — DescribeVpcEndpoints mocks base method.
- `MockClient.DescribeVpcEndpointsPages` — DescribeVpcEndpointsPages mocks base method.
- `MockClient.DescribeVpcPeeringConnections` — DescribeVpcPeeringConnections mocks base method.
- `MockClient.DescribeVpcs` — DescribeVpcs mocks base method.
- `MockClient.DisassociateVPCFromHostedZone` — DisassociateVPCFromHostedZone mocks base method.
- `MockClient.EXPECT` — EXPECT returns an object that allows the caller to indicate expected use.
- `MockClient.GetCallerIdentity` — GetCallerIdentity mocks base method.
- `MockClient.GetHostedZone` — GetHostedZone mocks base method.
- `MockClient.GetResourcesPages` — GetResourcesPages mocks base method.
- `MockClient.ListHostedZonesByName` — ListHostedZonesByName mocks base method.
- `MockClient.ListHostedZonesByVPC` — ListHostedZonesByVPC mocks base method.
- `MockClient.ListResourceRecordSets` — ListResourceRecordSets mocks base method.
- `MockClient.ListTagsForResource` — ListTagsForResource mocks base method.
- `MockClient.ModifyVpcEndpointServiceConfiguration` — ModifyVpcEndpointServiceConfiguration mocks base method.
- `MockClient.ModifyVpcEndpointServicePermissions` — ModifyVpcEndpointServicePermissions mocks base method.
- `MockClient.RevokeSecurityGroupIngress` — RevokeSecurityGroupIngress mocks base method.
- `MockClient.StartInstances` — StartInstances mocks base method.
- `MockClient.StopInstances` — StopInstances mocks base method.
- `MockClient.TerminateInstances` — TerminateInstances mocks base method.
- `MockClient.Upload` — Upload mocks base method.
- `MockClient.WaitUntilVpcPeeringConnectionDeleted` — WaitUntilVpcPeeringConnectionDeleted mocks base method.
- `MockClient.WaitUntilVpcPeeringConnectionExists` — WaitUntilVpcPeeringConnectionExists mocks base method.
- `MockClientMockRecorder` — MockClientMockRecorder is the mock recorder for MockClient.
- `MockClientMockRecorder.AcceptVpcPeeringConnection` — AcceptVpcPeeringConnection indicates an expected call of AcceptVpcPeeringConnection.
- `MockClientMockRecorder.AssociateVPCWithHostedZone` — AssociateVPCWithHostedZone indicates an expected call of AssociateVPCWithHostedZone.
- `MockClientMockRecorder.AuthorizeSecurityGroupIngress` — AuthorizeSecurityGroupIngress indicates an expected call of AuthorizeSecurityGroupIngress.
- `MockClientMockRecorder.ChangeResourceRecordSets` — ChangeResourceRecordSets indicates an expected call of ChangeResourceRecordSets.
- `MockClientMockRecorder.ChangeTagsForResource` — ChangeTagsForResource indicates an expected call of ChangeTagsForResource.
- `MockClientMockRecorder.CreateHostedZone` — CreateHostedZone indicates an expected call of CreateHostedZone.
- `MockClientMockRecorder.CreateRoute` — CreateRoute indicates an expected call of CreateRoute.
- `MockClientMockRecorder.CreateVPCAssociationAuthorization` — CreateVPCAssociationAuthorization indicates an expected call of CreateVPCAssociationAuthorization.
- `MockClientMockRecorder.CreateVpcEndpoint` — CreateVpcEndpoint indicates an expected call of CreateVpcEndpoint.
- `MockClientMockRecorder.CreateVpcEndpointServiceConfiguration` — CreateVpcEndpointServiceConfiguration indicates an expected call of CreateVpcEndpointServiceConfiguration.
- `MockClientMockRecorder.CreateVpcPeeringConnection` — CreateVpcPeeringConnection indicates an expected call of CreateVpcPeeringConnection.
- `MockClientMockRecorder.DeleteHostedZone` — DeleteHostedZone indicates an expected call of DeleteHostedZone.
- `MockClientMockRecorder.DeleteRoute` — DeleteRoute indicates an expected call of DeleteRoute.
- `MockClientMockRecorder.DeleteVPCAssociationAuthorization` — DeleteVPCAssociationAuthorization indicates an expected call of DeleteVPCAssociationAuthorization.
- `MockClientMockRecorder.DeleteVpcEndpointServiceConfigurations` — DeleteVpcEndpointServiceConfigurations indicates an expected call of DeleteVpcEndpointServiceConfigurations.
- `MockClientMockRecorder.DeleteVpcEndpoints` — DeleteVpcEndpoints indicates an expected call of DeleteVpcEndpoints.
- `MockClientMockRecorder.DeleteVpcPeeringConnection` — DeleteVpcPeeringConnection indicates an expected call of DeleteVpcPeeringConnection.
- `MockClientMockRecorder.DescribeAvailabilityZones` — DescribeAvailabilityZones indicates an expected call of DescribeAvailabilityZones.
- `MockClientMockRecorder.DescribeInstances` — DescribeInstances indicates an expected call of DescribeInstances.
- `MockClientMockRecorder.DescribeLoadBalancers` — DescribeLoadBalancers indicates an expected call of DescribeLoadBalancers.
- `MockClientMockRecorder.DescribeNetworkInterfaces` — DescribeNetworkInterfaces indicates an expected call of DescribeNetworkInterfaces.
- `MockClientMockRecorder.DescribeRouteTables` — DescribeRouteTables indicates an expected call of DescribeRouteTables.
- `MockClientMockRecorder.DescribeRouteTablesPages` — DescribeRouteTablesPages indicates an expected call of DescribeRouteTablesPages.
- `MockClientMockRecorder.DescribeSecurityGroups` — DescribeSecurityGroups indicates an expected call of DescribeSecurityGroups.
- `MockClientMockRecorder.DescribeSubnets` — DescribeSubnets indicates an expected call of DescribeSubnets.
- `MockClientMockRecorder.DescribeSubnetsPages` — DescribeSubnetsPages indicates an expected call of DescribeSubnetsPages.
- `MockClientMockRecorder.DescribeVpcEndpointServiceConfigurations` — DescribeVpcEndpointServiceConfigurations indicates an expected call of DescribeVpcEndpointServiceConfigurations.


*(Showing first 80 of 110 merged export names.)*

## Internal Dependencies

- `context`
- `github.com/aws/aws-sdk-go-v2/feature/s3/manager`
- `github.com/aws/aws-sdk-go-v2/service/ec2`
- `github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2`
- `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi`
- `github.com/aws/aws-sdk-go-v2/service/route53`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/aws/aws-sdk-go-v2/service/sts`
- `go.uber.org/mock/gomock`
- `reflect`

## Capabilities

- **`package`** name(s): **mock**.
- Go **`import`** edges listed below (10 unique path(s)).
- Package ID(s): `github.com/openshift/hive/pkg/awsclient/mock`.

## Understanding Score

0.0

# AWS Private Link

## Overview

Customers often want the core services of their OpenShift cluster to be
available only on the internal network and not on the Internet. The API server
is one such service that customers do not want to be accessible over the
Internet. The OpenShift Installer allows creating clusters that have their
services published only on the internal network by setting `publish: Internal`
in the install-config.yaml.

Since Hive is usually running outside the network where the clusters exists, it
requires access to the cluster's API server which mostly translates to having
the API reachable over the Internet. There can be some restrictions setup to
allow only Hive to access the API but these are usually not acceptable by
security focused customers.

AWS provides a feature called AWS Private Link ([see doc][aws-private-link-doc]) that allows
accessing private services in customer VPCs from another account using AWS's internal
networking and not the Internet. AWS Private Link involves creating a VPC
Endpoint Service in customer's account that is backed by one or more internal
NLBS and a VPC Endpoint in service provider's account. This allows clients in
service provider's VPC to access the NLB backed service in customer's account
using the endpoint-endpoint service Private Link. So the internal service is
now accessible to the service provider without exposing the service to the
Internet.

Using this same architecture, we create a VPC Endpoint Service, VPC Endpoint
pair to create an Private Link to cluster's internal NLB for k8s API server,
allowing Hive to access the API without forcing the cluster to publish it on
the Internet.

## Configuring Hive to enable AWS Private Link

To configure Hive to support Private Link in a specific region,

1. Create VPCs in that region that can be used to create VPC Endpoints.

    NOTE: There is a hard limit of 255 VPC Endpoints in a region, therefore you
    will need multiple VPCs to support more cluster in that region.

2. For each VPC, create subnets in all the supported availability zones of the
  region.

    NOTE: each subnet must have at least 255 usuable IPs because the controller.

    For example let's create VPCs in us-east-1 region, that has 6 AZs.

    ```txt
    vpc-1 (us-east-1) : 10.0.0.0/20
      subnet-11 (us-east-1a): 10.0.0.0/23
      subnet-12 (us-east-1b): 10.0.2.0/23
      subnet-13 (us-east-1c): 10.0.4.0/23
      subnet-12 (us-east-1d): 10.0.8.0/23
      subnet-12 (us-east-1e): 10.0.10.0/23
      subnet-12 (us-east-1f): 10.0.12.0/23
    ```

    ```txt
    vpc-2 (us-east-1) : 10.0.16.0/20
      subnet-21 (us-east-1a): 10.0.16.0/23
      subnet-22 (us-east-1b): 10.0.18.0/23
      subnet-23 (us-east-1c): 10.0.20.0/23
      subnet-24 (us-east-1d): 10.0.22.0/23
      subnet-25 (us-east-1e): 10.0.24.0/23
      subnet-26 (us-east-1f): 10.0.28.0/23
    ```

3. Make sure all the Hive environments (Hive VPCs) have network reachability to
  these VPCs created above for VPC Endpoints using peering, transit gateways, etc.

4. Gather a list of VPCs that will need to resolve the DNS setup for Private
  Link. This should at least include the VPC of the Hive being configured, and
  can include list of all VPCs where various Hive controllers exists.

5. Update the HiveConfig to enable Private Link for clusters in that region.

    ```yaml
    ## hiveconfig
    spec:
      awsPrivateLink:
        ## this this is list if inventory of VPCs that can be used to create VPC
        ## endpoints by the controller
        endpointVPCInventory:
        - region: us-east-1
          vpcID: vpc-1
          subnets:
          - availabilityZone: us-east-1a
            subnetID: subnet-11
          - availabilityZone: us-east-1b
            subnetID: subnet-12
          - availabilityZone: us-east-1c
            subnetID: subnet-13
          - availabilityZone: us-east-1d
            subnetID: subnet-14
          - availabilityZone: us-east-1e
            subnetID: subnet-15
          - availabilityZone: us-east-1f
            subnetID: subnet-16
        - region: us-east-1
          vpcID: vpc-2
          subnets:
          - availabilityZone: us-east-1a
            subnetID: subnet-21
          - availabilityZone: us-east-1b
            subnetID: subnet-22
          - availabilityZone: us-east-1c
            subnetID: subnet-23
          - availabilityZone: us-east-1d
            subnetID: subnet-24
          - availabilityZone: us-east-1e
            subnetID: subnet-25
          - availabilityZone: us-east-1f
            subnetID: subnet-26

        ## credentialsSecretRef points to a secret with permissions to create
        ## resources in account where the inventory of VPCs exist.
        credentialsSecretRef:
          name: < hub-account-credentials-secret-name >

        ## this is a list of VPC where various Hive clusters exists.
        associatedVPCs:
        - region: region-hive1
          vpcID: vpc-hive1
          credentialsSecretRef:
            name: < credentials that have access to account where Hive1 VPC exists >
        - region: region-hive2
          vpcID: vpc-hive2
          credentialsSecretRef:
            name: < credentials that have access to account where Hive2 VPC exists>
    ```

    You can include VPC from all the regions where private link is supported in the
    endpointVPCInventory list. The controller will pick a VPC appropriate for the
    ClusterDeployment.

### Security Groups for VPC Endpoints

Each VPC Endpoint in AWS has a Security Group attached to control access to the endpoint.
See the [docs][control-access-vpc-endpoint] for details.

When Hive creates VPC Endpoint, it does not specify any Security Group and therefore the
default Security Group of the VPC is attached to the VPC Endpoint. Therefore, the default
security group of the VPC where VPC Endpoints are created must have rules to allow traffic
from the Hive installer pods.

For example, if Hive is running in hive-vpc(10.1.0.0/16), there must be a rule in default
Security Group of VPC where VPC Endpoint is created that allows ingess from 10.1.0.0/16.

## Using AWS Private Link

Once Hive is configured to support Private Link for AWS clusters, customers can
create ClusterDeployment objects with Private Link by setting the
`privateLink.enabled` to `true` in `aws` platform. This is only supported in
regions where Hive is configured to support Private Link, the validating
webhooks will reject ClusterDeployments that request private link in
unsupported regions.

```yaml
spec:
  platform:
    aws:
      privateLink:
        enabled: true
```

The controller provides progress and failure updates using `AWSPrivateLinkReady` and
`AWSPrivateLinkFailed` conditions on the ClusterDeployment.

## Permissions required for AWS Private Link

There multiple credentials involved in the configuring AWS Private Link and there are different
expectations of required permissions for these credentials.

1. The credentials on ClusterDeployment

    The following permissions are required:

    ```txt
    ec2:CreateVpcEndpointServiceConfiguration
    ec2:DescribeVpcEndpointServiceConfigurations
    ec2:ModifyVpcEndpointServiceConfiguration
    ec2:DescribeVpcEndpointServicePermissions
    ec2:ModifyVpcEndpointServicePermissions

    ec2:DeleteVpcEndpointServiceConfigurations
    ```

2. The credentials specified in HiveConfig for endpoint VPCs account `.spec.awsPrivateLink.credentialsSecretRef`

    The following permissions are required:

    ```txt
    ec2:DescribeVpcEndpointServices
    ec2:DescribeVpcEndpoints
    ec2:CreateVpcEndpoint
    ec2:CreateTags
    ec2:DescribeVPCs

    ec2:DeleteVpcEndpoints

    route53:CreateHostedZone
    route53:GetHostedZone
    route53:ListHostedZonesByVPC
    route53:AssociateVPCWithHostedZone
    route53:DisassociateVPCFromHostedZone
    route53:CreateVPCAssociationAuthorization
    route53:DeleteVPCAssociationAuthorization
    route53:ListResourceRecordSets
    route53:ChangeResourceRecordSets

    route53:DeleteHostedZone
    ```

3. The credentials specified in HiveConfig for associating VPCs to the Private Hosted Zone.
  `.spec.awsPrivateLink.associatedVPCs[$idx].credentialsSecretRef`

    The following permissions are required in the account where the VPC exists:

    ```txt
    route53:AssociateVPCWithHostedZone
    route53:DisassociateVPCFromHostedZone
    ec2:DescribeVPCs
    ```

[aws-private-link-overview]: https://docs.aws.amazon.com/vpc/latest/privatelink/endpoint-services-overview.html
[control-access-vpc-endpoint]: https://docs.aws.amazon.com/vpc/latest/privatelink/vpc-endpoints-access.html#vpc-endpoints-security-groups

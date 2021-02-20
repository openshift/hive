# Private clusters on AWS using PrivateLink

## Summary

Hive creates a private AWS cluster using OpenShift Installer and then sets up
exclusive access to the private cluster using AWS PrivateLink. Hive creates an
VPC Endpoint Service in customer's AWS account for the cluster's k8s API and an
VPC Endpoint in Hub cluster to access the cluster's k8s API privately.

## Motivation

### Goals

- Support creating private cluster with no access from the public Internet using Hive.

### Non-Goals

-

## Proposal

Create a new controller that can identify a ClusterDeployment that requires
AWS PrivateLink access to the k8s API. The controller uses the ClusterMetadata
and Credentials for the cluster to create a VPC Endpoint Service (AWS PrivateLink)
for interface endpoints using the NLB created by the OpenShift Installer. The controller
then creates an Interface Endpoint in a Hub AWS account so that Hive has access to the
cluster's k8s API using PrivateLink.

### User Stories

#### Story 1

#### Story 2

## Design Details

### DNS resolution for installer pods to use PrivateLink

see: https://aws.amazon.com/blogs/networking-and-content-delivery/integrating-aws-transit-gateway-with-aws-privatelink-and-amazon-route-53-resolver/
This blog describes how **private DNS** can be setup for VPC endpoints so that actors
in the VPC where the VPC endpoint exists (VPCEndpointVPC) and VPCs that are peered to
the VPCEndpointVPC can resolve the DNS to network address of the VPC Endpoint.

This is very useful for overriding resolution of the *default* DNS name for a cluster's
API server to that of the VPC Endpoint and allows installer pods to contact the cluster's
API server using the *default* DNS names using PrivateLink instead of the public
Internet.

Hive creates a Private Hosted Zone (PHZ) for the *default* API server URL and alias the
DNS record to point to the VPC Endpoint created for the cluster. It then associates the
PHZ to the VPC where the installer pods are running. The installer can reach the cluster's
API server using PrivateLink and continue with installation.

Hive also ensures that the PHZ is associated with it's VPC too so that it can also continue
to connect with the cluster for all day-2 operations.

### Approving VPC Endpoints

Endpoints services can require that any endpoints created to the service be
automatically accepted or manually approved. Now since, these endpoint services
are only visible (or accessible) to users defined by the AllowedPrincipals list for
the service if only very specific user/role/account is allowed to create
endpoints for these services, using automatic approval is a safe option.

```shell
aws ec2 create-vpc-endpoint-service-configuration --no-acceptance-required
```

```shell
aws ec2 modify-vpc-endpoint-service-permissions \
  --add-allowed-principals '["arn:aws:iam::123456789012:root"]' 
```

Effectively only allow users in 123456789012 account to create endpoints to the service.
NOTE: This can be restricted to a specific user/role in the account too.

### Manual walk-through

Here is a manual walk through of all the steps that need to be taken today
to setup the required environment using just OpenShift installer.

Use the ignition-configs target for extracting necessary information about the cluster.

```shell
$ openshift-install --dir install_dir01 create ignition-configs
```

Run the installer to create the cluster

```shell
$ openshift-install --dir install_dir01 create cluster
INFO Credentials loaded from the "default" profile in file "/home/user/.aws/credentials"
INFO Consuming Install Config from target directory
INFO Ignition-Configs created in: dev and dev/auth
$ cat install_dir01/metadata.json | jq .infraID
"user-sc885"
$ yq e '.clusters[0].cluster.server' install_dir01/auth/kubeconfig
https://api.user.devcluster.openshift.com:6443
```

At this stage the installer has created the NLB for the apiserver and the
installation is already in progress in the bootstrap ec2instance. We can use
the installer tags to figure out the apiserver's internal NLB for the cluster.

```shell
AWS_PROFILE=customer aws elbv2 describe-load-balancers --names "{infra_id}-int"
```

Create an VPC Endpoint Service in AWS account of the cluster.

```shell
AWS_PROFILE=customer aws ec2 create-vpc-endpoint-service-configuration \
--no-acceptance-required \
--network-load-balancer-arns "{nlb_arn}"
```

Get the identity of the user in Hub AWS account so that we can use the VPC
Endpoint Service.

```shell
AWS_PROFILE=hub01 aws sts get-caller-identity
```

Add the user to the allowed principals of the VPC Endpoint Service.

```shell
AWS_PROFILE=customer aws ec2 modify-vpc-endpoint-service-permissions \
--add-allowed-principals '["{hub_user_arn}"]' 
```

Now that the VPC Endpoint Service has been created. Next create an VPC Endpoint of
type `Interface` in the AWS Hub account.

Get the VPC Endpoint Service details for availability zones.

```shell
$ AWS_PROFILE=hub01 aws ec2 describe-vpc-endpoint-services \
--service-names <service name>
$ SERVICE_AVAILABITY_ZONES=$(jq '.ServiceDetails[0].AvailabilityZones')
```

```shell
$ SUBNETS=$(subnetsForAZs SERVICE_AVAILABITY_ZONES)
$ AWS_PROFILE=hub01 aws ec2 create-vpc-endpoint \
--vpc-id <vpc-id> \
--vpc-endpoint-type Interface \
--service-name <service name> \
--subnet-ids $SUBNETS \
--security-group-ids <security group that allows ingress> \
```

We did not enable the private DNS support for the VPC Endpoint so that we can control
our own resolution using a Private Hosted Zone (PHZ). Lets create a PHZ for the cluster's
default API server DNS name and associate it with the VPC where the installer is running.

```shell
AWS_PROFILE=hub01  aws route53 create-hosted-zone \
  --hosted-zone-config Comment="PHZ for cluster",PrivateZone=true \
  --vpc VPCId="VPC Id for the VPC Endpoint",VPCRegion="Region of the VPC" \
  --name api.user.devcluster.openshift.com \
  --caller-reference uniqueid
```

Create a record in the PHZ ALIAS to the *regional DNS name* of the VPC Endpoint.

```shell
AWS_PROFILE=hub01 aws route53 change-resource-record-sets --hosted-zone-id "id of the hosted zone" --changes 'shown below'
```

```json
{
  "Comment": "Add Alias record to the VPC endpoint",
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "api.user.devcluster.openshift.com",
        "Type": "A",
        "TTL": 10,
        "AliasTarget": {
          "HostedZoneId": "HZ12345678",
          "DNSName": "vpce-1234567890-1234567890.vpce-svc-1234567890.us-east-1.vpce.amazonaws.com",
          "EvaluateTargetHealth": false
        }
      }
    }
  ]
}
```

Now associate the PHZ to the VPC where the installer is running.

```shell
AWS_PROFILE=hub01 aws route53 associate-vpc-with-hosted-zone \
  --hosted-zone-id "id of the hosted zone" \
  --vpc VPCId="id of the VPC where installer is running",VPCRegion="region of the vpc"
```

The installer should now finally be able to communicate with the cluster's k8s API,
and continue with the next stages of waiting for bootstrapping to complete etc.

### AWS Private Link Controller

The goal of the controller to manage a PrivateLink for the cluster as required
by the ClusterDeployment. It watches the ClusterProvision for a ClusterDeployment
to create necessary resources and ClusterDeprovision to clean the resources.

#### API change

The controller requires various information about the cluster and the environment
to manage the PrivateLink resources.

```yaml
# clusterdeployment
spec:
  platform:
    aws:
      privateLink:
        # when set to true, PrivateLink will be setup for communication during
        # installation and day-2 operations with the cluster.
        enabled: boolean
        
        vpcEndpointService:
          # this is ServiceName for the vpc endpoint service
          name: (string)
          # this is the ServiceID for the vpc endpoint service
          id: (string)
        # this is the ID for the vpc endpoint
        vpcEndpointID: (string)
        # this is the ID for the private hosted zone that is created
        # to provide dns resolution to cluster's default apiserver
        # using PrivateLink
        hostedZoneID: (string)
```

NOTE: The fields `vpcEndpointService`, `vpcEndpointID`, `hostedZoneID` should ideally be
stored in the status for the ClusterDeployment since these resources are not provided by
the user but created by the controller for the cluster. But since these fields cannot be
recomputed by the controller after recovery, these are stored in spec.

```yaml
# hiveconfig
spec:
  awsPrivateLinkController:
    # this secret points to credentials that will be used to create resources
    # in the HUB account. 
    credentialsSecretRef:
      name: string

    # this defines a list of VPCs in all the supported regions that
    # can be used to create vpc endpoints.
    # The expectation is that these vpcs have network connectivity to the
    # cluster where hive is running.
    vpcEndpointInventory:
    - region: string
      vpcID: string
      subnets: list(string)

    # this defines a list of VPCs that will be able to resolve the
    # records in private hosted zones for the VPC endpoints.
    # The VPC for cluster where this hive is running must be part of
    # this list. Additional VPCs can be added here like VPCs for
    # other clusters where hives are running so that there is easy
    # movement of clusterdeployments among instances of hive.
    assosiatedVPCs:
    - region: string
      vpcID: string

```

#### Discovering NLB for cluster

To create the VPC Endpoint Service for the cluster, the controller requires the ARN
to the Network Loadbalancer for the internal API. This load balancer is created
by the installer with the name `{infra_id}-int`.

The controller uses `.spec.infraID` from the ClusterProvision that is in progress for
a ClusterDeployment to get indentifier and the credentials from `.spec.platform.aws.credentialsSecretRef`
Secret to authenticate with AWS.

see doc for [api reference](https://docs.aws.amazon.com/elasticloadbalancing/2012-06-01/APIReference/API_DescribeLoadBalancers.html)
```
elbv2.DescribeLoadBalancers
  LoadBalancerNames:
  - "{.spec.infraID}-int"
```

#### Creating VPC Endpoint Service

Once we have the Arn for the NLB, we need

- allowed IAM principals for the service
The IAM pricipal that is allowed to access and create endpoints to the service
will be the entity specified in `.spec.awsPrivateLinkController.credentialsSecretRef` in HiveConfig.
The controller can use [STS's GetCallerIdentity](https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html)
to figure out the Arn for the AWS entity.

The controller continues to create the VPC Endpoint Service in cluster's account using

see this doc for [api reference](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVpcEndpointServiceConfiguration.html)
```
ec2.CreateVpcEndpointServiceConfiguration
  AcceptanceRequired: False
  NetworkLoadBalancerArn:
  - "{Arn for the cluster's loadbalancer}"
```

The controller stores the `serviceName` and `serviceID` from the response of CreateVpcEndpointServiceConfiguration
to the ClusterDeployment `spec.platform.aws.privateLink.vpcEndpointService.name` and
`spec.platform.aws.privateLink.vpcEndpointService.id` respectively.

Next, the controller needs to add the IAM principals to the service configuration.
This cannot be set during creation of the service but needs to be modified using

see this doc for [api reference](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyVpcEndpointServicePermissions.html)
```
ec2.ModifyVpcEndpointServicePermissions
  AddAllowedPrincipals:
  - "{identity ARN for the hub account}"
  ServiceId: "privateLink.vpcEndpointService.id"
```

#### Calculating the VPC and Subnets for VPC Endpoint

The controller needs to figure out which VPC and subnets to use from the inventory
specified in HiveConfig at `.spec.awsPrivateLinkController.vpcEndpointInventory`.

1. The controller filters the list to only VPCs that in the same region as the
  VPC Endpoint Service.

2. The controller then filters out VPCs which don't have at least one subnet in the
  supported Availability Zones for the VPC Endpoint Service.

    The subnets that should be used depend on the AZs that are available for the VPC Endpoint
    Service. To figure out which AZs are supported by the service, the controller
    needs to describe the endpoint service from the context of the hub account.

    see the doc for [api reference](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcEndpointServices.html)
    ```
    ec2.DescribeVpcEndpointServices
      ServiceName:
      - "{privateLink.vpcEndpointService.name}"
    ```

    The response includes a list of avaialabilty zones. These are the friendly names
    for the AZs in hub account.
    NOTE: this list can be different from the cluster account as friendly names are
    different in different accounts.

3. The controller figures out which VPCs has available quota for a VPC Endpoint

    The controller list for all the VPC Endpoints in a region and computes the number
    of VPC Endpoints for each VPC. The controller then picks the fisrt VPC which has
    at least one available slot (VPC_ENDPOINT_LIMIT - available).

4. The controller then uses the subnets that match the supported list of AZs by the
  VPC Endpoint Service.

#### Creating VPC Endpoint

The controller can then create VPC endpoint using the following API,

see the doc for [api reference](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVpcEndpoint.html)
```
ec2.CreateVpcEndpoint
  ServiceName: "{serviceName}"
  SubnetId:
  - subnet-1
  - subnet-2 or more
  VpcId: "{computed VPC id from subnets}"
  VpcEndpointType: Interface
```

The controller stores the `Id` for the response of CreateVpcEndpoint to the ClusterDeployment
`spec.platform.aws.privateLink.vpcEndpointID`

The VPC Endpoint Service is confiured to automatically approve any VPC Endpoint request
created by the principals in allowed IAM Principals list. So the controller doesn't need to
take additional steps but since approval can take some time, it needs to wait for approval.

The controller can wait for approval using
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcEndpoints.html
```
ec2.DescribeVpcEndpoints
  VpcEndpointId:
  - "{privateLink.vpcEndpointId}"
```

The controller ensures that the state of the VPC Endpoint is `Available`.

#### DNS resolution for VPC Endpoint

Since the VPC Endpoint created by the controller did not turn on PrivateDNS support, the
controller needs to manage a Private Hosted Zone to provide DNS resolution to the
endpoint.

Managing a Private Hosted Zone has many advantages for Hive's environenment as specified
in [previous section][]

1. Compute the default address for cluster's API server

    The admin kubeconfig uploaded by the installmanager during a provisioning can
    be used to get the address.

    Read the kubeconfig stored in Secret provided by `.spec.adminKubeconfigSecretRef` on
    the ClusterProvision object when getting the infra identifier.

    `raw-kubeconfig` field in the Secret has the kubeconfig as created by the installer
    that has not been modified by the Hive.

    The address for the api server is stored in `.clusters[0].cluster.server` field in the
    kubeconfig.

2. Create a Private Hosted Zone in AWS Hub Account.

    The controller then created a Private Hosted Zone for the apiserver address.

    See the doc for [api reference](https://docs.aws.amazon.com/Route53/latest/APIReference/API_CreateHostedZone.html)
    ```
    route53.CreateHostedZone
      Name: "address for the apiserver"
      VPC:
        VPCId: "VPC where the endpoint was created"
        VPCRegion: "region for the VPC"
      HostedZoneConfig:
        PrivateZone: true
    ```

    Store the HostedZoneId from the response of CreateHostedZone to `privateLink.hostedZoneId` on
    the ClusterDeployment.

3. Create an ALIAS record in the hosted zone so that default apiserver address resolves to
  the VPC Endpoint.

    The alias target is the *regional DNS name* of the VPC Endpoint which is of the form
    `{vpc-endpoint-id}.{vpc-endpoint-service-id}.{region}.vpce.amazonaws.com`. The response
    of the CreateVPCEndpoint includes the list of DNS names and the controller can pick the
    entry that matches this format.

    ```json
    {
      "Comment": "Add Alias record to the VPC endpoint",
      "Changes": [
        {
          "Action": "UPSERT",
          "ResourceRecordSet": {
            "Name": "{address of the apiserver}",
            "Type": "A",
            "TTL": 10,
            "AliasTarget": {
              "HostedZoneId": "{hosted zone id of the regional DNS name of the VPC Endpoint}",
              "DNSName": "{regional DNS name of the VPC Endpoint}",
              "EvaluateTargetHealth": false
            }
          }
        }
      ]
    }
    ```

4. Create VPC associations to the PHZ so that things in Hive cluster can resolve the cluster
  apiserver DNS to the VPC Endpoint.

    For each VPC in `awsPrivateLinkController.associatedVPCs` in the HiveConfig, run the following
    AWS API call,

    see the doc for [api reference](https://docs.aws.amazon.com/Route53/latest/APIReference/API_AssociateVPCWithHostedZone.html)
    ```
    route53.AssociateVPCWithHostedZone
      Id: {privateLink.hostedZoneID}
      VPC:
        VPCId: "{awsPrivateLinkController.associatedVPCs[$idx].vpcID}"
        VPCRegion: "{awsPrivateLinkController.associatedVPCs[$idx].region}"
    ```

#### Failed installations

Since the installations can fail, the endpoint service pointed to NLB will prevent
it's deletion and therefore to retry installation using the destroy the cluster and
retry again requires deleting the endpoint, endpoint service and in that order.

Currently when an installation fails, the new ClusterProvision uses the identifiers
from the old provision to destroy the cluster and then continue with trying to
install the cluster.

Since the controller already watches for ClusterProvision object, whenever the controller
see a ClusterProvision object that is progressing and has `.spec.prevInfraID` set, it
uses the fields in `.spec.platform.aws.privateLink` of the corresponding ClusterDeployment
to delete any resources that were created during the previous provision.

for a ClusterProvision object:
  if `.spec.stage` is not in ("complete", "failed"):
    if `.spec.prevInfraID` is set:
      trigger the deprovision of private link with (`.spec.prevInfraID`, `.spec.clusterDeploymentRef`)

Since the new installmanager cannot proceed without the VPC Endpoint Service being deleted,
the installer will automatically retry trying to clean up all the resource until the controller
has finished the cleanup. When the destroy succeeds, the installmanager will continue with
installation.

The controller can then push the ClusterProvision object into queue again to create the
new resources for PrivateLink for this current attempt whenever the `.spec.infraID` is
available.

#### Deprovisioning PrivateLink

Whenever the controller sees a ClusterDeprovision object for a ClusterDeployment that had
PrivateLink enbaled, it must clean up the resources so that installmanager can succeed in
destroying the cluster resources.

1. Delete the Private Hosted Zone in Hub AWS Account

  When the `privateLink.hostedZoneID` is set, the controller uses the `route53.DeleteHostedZone`
  AWS API to delete the PHZ. This uses `awsPrivateLinkController.credentialsSecretRef`
  from the HiveConfig for authorization.

2. Delete the VPC Endpoint in Hub AWS Account

  When the `privateLink.vpcEndpointID` is set, the controller uses the `ec2.DeleteVpcEndpoints`
  AWS API to delete the VPC Endpoint. This uses `awsPrivateLinkController.credentialsSecretRef`
  from the HiveConfig for authorization.

3. Delete the VPC Endpoint Service in cluster's AWS Account

  When the `privateLink.vpcEndpointService.ID` is set, the controller uses the `ec2.DeleteVpcEndpointServiceConfigurations`
  AWS API to delete the VPC Endpoint Service. This uses `.spec.platform.aws.credentialsSecretref`
  from the ClusterDeployment for authorization.

### Risks and Mitigations

### Test Plan

## Alternatives

### EC2instance in user account to run installer

### installer that can create infrastructure without waiting

### New managed NLB instead of using installer created one

# Private clusters on AWS using PrivateLink

## Summary

Hive creates a private AWS cluster using OpenShift Installer and then sets up
exclusive access to the private cluster using AWS PrivateLink. Hive creates an
Endpoint Service in customer's AWS account for the cluster's k8s API and an
Endpoint in Hub cluster to access the cluster's k8s API privately.

## Motivation

### Goals

- Support creating private cluster with no access from Internet using Hive.

### Non-Goals

-

## Proposal

Create a new controller that can identify a ClusterDeployment that requires
AWS PrivateLink access to the k8s API. This controller uses the ClusterMetadata
and credentials for the cluster to identify NLB for the cluster, setup an
Endpoint Service to expose the NLB to *specific* AWS role in Hub account, setup
and Endpoint to the customer's Service in Hub account and finally make the endpoint
DNS names available for use.

Hive users the OpenShift Installer to *ONLY* create infrastructure resources and
then waits for the controller to report the private DNS names for the Endpoint
service. After the private DNS names are available, continue the install to wait
for API to come up, destroy the bootstrap resources and wait for install to complete
using the pre-existing openshift-installer subcommands. 

Hive also updates the manifests for API server to accept the connections from 
the Endpoint service DNS names at installation.

### User Stories

#### Story 1

#### Story 2

## Design Details

### Private DNS names for endpoint services

doc: https://docs.aws.amazon.com/vpc/latest/userguide/verify-domains.html

When you create a VPC endpoint service, AWS generates endpoint-specific DNS
hostnames that you can use to communicate with the service. These names include
the VPC endpoint ID, the Availability Zone name and Region Name, for example, 
vpce-1234-abcdev-us-east-1.vpce-svc-123345.us-east-1.vpce.amazonaws.com.

For successful communication with API using **this** DNS name, the apiserver must
be configured to serve the appropriate ServerCertifcate and client-go clients
must be configured with the corresponding TrustCertificate in form of the kubeconfig.
This configuration must be modified day-1 (during installation) because we
need a valid working SNI configuration to exist before we can communicate with the
apiserver to update it.

So to solve the chicken-egg problem, we must use user-defined private DNS names
for Endpoint services that are easily computed before we begin the installation.
This requires a **public** HostedZone to allow AWS to perform Domain ownership
validations for the DNS name.

For each such installation, we use the unique set of inputs like namespace, name,
cluster name etc. and hash it to RFC 1034 DNS label space. We then use the hash 
to create the private DNS for cluster's endpoint service like,
`api.<hash>.<hosted zone domain>`.

When an endpoint service is created in the customer's account with this DNS name,
TXT records are created in the HostedZone to verify the domain. When an endpoint
is created to the service in hub account, all subnets in the chosen VPC will be
able to resolve this DNS address to the endpoint service.

### Setup additional DNS name for apiserver at installation

To configure an additional DNS name for apiserver we require,
- the DNS name
- a trust authority
- a server certificate signed by that trust authority
- Secret object with server certificate and key
- APIServer object pointing to Secret

For simplicity's sake, let's assume that all apiservers will be configured
with the same ServerCertificate `*.<hosted zone domain>`.

We add the APIServer and Secret object with correct configuration during the
manifests stage of openshift-install and the cluster apiserver will setup the
listeners to respond correctly.

### Approving VPC Endpoints

Endpoints services can require that any endpoints created to the service be
automatically accepted or manually approved. Now since, these endpoint services
are only visible (or accessible) to users defined by the AllowedPrincipals list for
the service if only very specific user/role/account is allowed to create
endpoints for these services, using automatic approval is a safe option.

```shell=bash
$ aws ec2 create-vpc-endpoint-service-configuration --no-acceptance-required
```
```shell=bash
$ aws ec2 modify-vpc-endpoint-service-permissions \
--add-allowed-principals '["arn:aws:iam::123456789012:root"]' 
```

Effectively only allow users in 123456789012 account to create endpoints to the service.
NOTE: This can be restricted to a specific user/role in the account too.

### Manual walk-through

Here is a manual walk through of all the steps that need to be taken today
to setup the required environment.

So let's assume we have a public Route53 HostedZone `hub01.osdprivate.io`. And
the computes DNS name for the endpoint service is `api.hash123.hub01osdprivate.io`.

Create the manifests for install.
```shell=bash
$ openshift-install --dir install_dir01 create manifests
```

Add the APIServer and Secret object for setting up apiserver SNI.
```shell=bash
$ cat > install_dir01/manifests/apiserver.yaml << EOF
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  servingCerts:
    namedCertificates:
    - names:
      - api.hash123.hub01osdprivate.io
      servingCertificate:
       name: apiserver-private-link-ingress
EOF
$ cat > install_dir01/manifests/secret-apiserver-private-link-ingress.yaml << EOF
apiVersion: v1
kind: Secret
metadata:
  namespace: openshift-config
  name: apiserver-private-link-ingress
tls.key: <the serving certificate key here>
tls.crt: <the serving certificate here>
EOF
```

Create infrastructure resources only.
```shell=bash
$ openshift-install --dir install_dir01 create cluster --no-wait-only-infra
```

At this stage the installer has created the NLB for the apiserver and the
installation is already in progress in the bootstrap ec2instance. We can use
the installer tags to figure out the apiserver's internal NLB for the cluster.

Create an Endpoint service in AWS account of the cluster.
```shell=bash
$ AWS_PROFILE=customer aws ec2 create-vpc-endpoint-service-configuration \
--no-acceptance-required \
--private-dns-name api.hash123.hub01osdprivate.io \
--network-load-balancer-arns <nlb arn here>
```
```shell=bash
$ aws ec2 modify-vpc-endpoint-service-permissions \
--add-allowed-principals '["arn:aws:iam::123456789012:root"]' 
```

Extract the DNS verification information from the create.
```shell=bash
$ DOMAIN_VERIFICATION_NAME=$(jq '.ServiceConfigurations[0].PrivateDnsNameConfiguration.Name')
$ DOMAIN_VERIFICATION_VALUE=$(jq '.ServiceConfigurations[0].PrivateDnsNameConfiguration.Value')
```

Let's create the TXT records in HostedZone to complete the verification.
NOTE: This is happening in the hub AWS account.
```json
{
  "HostedZoneId": "$HOSTED_ZONE_ID",
  "ChangeBatch": {
    "Comment": "Add TXT record for domain verification",
    "Changes": [
      {
        "Action": "UPSERT",
        "ResourceRecordSet": {
          "Name": "$DOMAIN_VERIFICATION_NAME.api.hash123.hub01osdprivate.io",
          "Type": "TXT",
          "RecordSets": [{
            "Value": "$DOMAIN_VERIFICATION_NAME"
          }]
        }
      }
    ]
  }
}
```

Now trigger the verifiation of the domain name manually.
```shell=bash
$ AWS_PROFILE=customer aws ec2 start-vpc-endpoint-service-private-dns-verification
```

At this point the Endpoint service is ready for use. Next create an Endpoint
Interface in hub account.

Get the Endpoint Service details for availability zones.
```shell=bash
$ AWS_PROFILE=hub01 aws ec2 describe-vpc-endpoint-services \
--service-names <service name>
$ SERVICE_AVAILABITY_ZONES=$(jq '.ServiceDetails[0].AvailabilityZones')
```
```shell=bash
$ SUBNETS=$(subnetsForAZs SERVICE_AVAILABITY_ZONES)
$ AWS_PROFILE=hub01 aws ec2 create-vpc-endpoint \
--vpc-id <vpc-id> \
--vpc-endpoint-type Interface \
--service-name <service name> \
--subnet-ids $SUBNETS \
--security-group-ids <security group that allows ingress> \
--private-dns-enabled
```
NOTE: To use private DNS, you must set the following VPC attributes 
to true: enableDnsHostnames and enableDnsSupport.

Now the `api.hash123.hub01osdprivate.io:6443` from the hub01 VPC should use the
setup Endpoint Interface and Endpoint Service to reach the private API of customer's
cluster.

Update the kubeconfig used by the OpenShift installer to use new address.
```shell=bash
# modify the install_dir01/auth/kubeconfig
# replace api.clustername.basedomain:6443 => api.hash123.hub01osdprivate.io:6443
# replace the trust bundle to new trust
```

Now run the rest of the stages for the installer.
```shell=bash
$ openshift-install --dir install_dir01 wait-for bootstrap-complete
$ openshift-install --dir install_dir01 destroy bootstrap
$ openshift-install --dir install_dir01 wait-for install-complete
# install complete
```

Update the ClusterDeployment's kubeconfig used for remote calls to use the
new address and trust.

### AWS Private Link Controller

The goal of the controller to ensure a PrivateLink is established to the cluster
that is being created by a ClusterDeployment. The controller watches for
ClusterProvisioning and ClusterDeployments that require a PrivateLink and uses 
the information in the aws platform to set it up.

#### API change

The ClusterDeployment needs to include additional information for the controller
to create Privatelink. Some of the information that is required are,
- infrastructure identifier for the cluster to discover the Network Load Balancer
- private DNS name for the service
- User/Role in AWS account (hub account) where the endpoint is to be created
- subnets in AWS account (hub account) where the endpoint is to be created
- credentials to the AWS account (hub account)

```yaml=
# clusterdeployment
spec:
  platform:
    aws:
      privatLink:
        # this is the public r53 zone in hub account where the TXT records will be 
        # added for domain verification of endpoint service
        hostedZone: (string)
        # this is list of subnets in hub account for the endpoint
        # the subnets used for the endpoint depends on AZs supported by the
        # service (which depends on the AZs supported by the NLB)
        subnets: (list(string))
        # this is reference to a secret that provide credentials to create
        # necessary resources in the hub account.
        # This role/user is only IAM entity that will be allowed to create endpoints
        # to the service created for the cluster.
        credentialsSecretRef:
          name: (string)
        
        # this allows the controller to store the information in spec
        # so that these can survice recovery and adoption in some cases
        
        endpointService:
          # this is ServiceName for the endpoint service
          name: (string)
          # this is the ServiceID for the endpoint service
          id: (string)
        # this is the ID for the vpc endpoint
        endpoint: (string)
```

#### Discovering NLB for cluster

To create the endpoint service for the cluster, the controller requires the ARN
to the Network Loadbalancer for the internal API. This load balancer is created
by the installer with the name `{infra_id}-int`.

The infrastructure ID should be available in the ClusterDeployment `.specclusterMetadata.infraID`.

So the discover the NLB Arn, following request can be made using the cluster
credentials and the following AWS API,

https://docs.aws.amazon.com/elasticloadbalancing/2012-06-01/APIReference/API_DescribeLoadBalancers.html
```
elbv2.DescribeLoadBalancers
  LoadBalancerNames:
  - "{infra_id}-int"
```

#### Creating Endpoint Service

Once we have the Arn for the NLB, we need

- private DNS name for the service,
The DNS name for the service will be the apiserver override URL specified in the
ClusterDeployment `.spec.controlPlaneConfig.apiURLOverride`

- allowed IAM principals for the service
The IAM pricipal that is allowed to access and create endpoints to the service
will the entity specified in the `privateLink.credentialsSecretRef`.

https://docs.aws.amazon.com/STS/latest/APIReference/API_GetCallerIdentity.html
The controller can use STS's GetCallerIdentity to figure out the Arn for
the AWS entity.

The controller continues to create the Endpoint service in cluster's account using

https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVpcEndpointServiceConfiguration.html
```
ec2.CreateVpcEndpointServiceConfiguration
  AcceptanceRequired: False
  NetworkLoadBalancerArn:
  - "{Arn for the cluster's loadbalancer}"
  PrivateDnsName: "{apiOverrideURL}"
```
The controller stores the `serviceName` and `serviceID` to the ClusterDeployment.
Next, the controller needs to add the IAM principals to the service configuration.
This cannot be set during creation of the service but needs to be modified using

https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyVpcEndpointServicePermissions.html
```
ec2.ModifyVpcEndpointServicePermissions
  AddAllowedPrincipals:
  - "{identity ARN for the hub account}"
```

#### Verifying the Endpoint Service Domain

The controller then needs to verify the domain used for the private DNS,

1. Get the latest DomainName and DomainValue for verification
To get the latest values, the controller can describe the service configuration using
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcEndpointServiceConfigurations.html

```
ec2.DescribeVpcEndpointServiceConfigurations
  ServiceIds:
  - "{serviceId}"
```

And extract the `Name` and `Value` from the PrivateDNSConfiguration as defined in
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_PrivateDnsNameConfiguration.html

If the `State` for the configuration is `verified`, the controller can skip the
next steps.

2. Add TXT record to the public R53 Zone
Once the controller has the domain name and domain value, the controller creates
TXT records in the hosted zone `privateLink.hostedZone` in hub account.

https://docs.aws.amazon.com/Route53/latest/APIReference/API_ChangeResourceRecordSets.html
```
route53.ChangeResourceRecordSets
  Id: "{hosted zone Id}",
  "ChangeBatch": {
    "Comment": "Add TXT record for domain verification",
    "Changes": [
      {
        "Action": "UPSERT",
        "ResourceRecordSet": {
          "Name": "{DomainName}.{apiOverrideURL}",
          "Type": "TXT",
          "RecordSets": [{
            "Value": "{DomainValue}"
          }]
        }
      }
    ]
  }
}
```

3. Run manual verification for the domain
Afer the route53 zone was updated, the controller needs to manually trigger the
verification of the domain.

https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_StartVpcEndpointServicePrivateDnsVerification.html
```
ec2.StartVpcEndpointServicePrivateDnsVerification
  ServiceId: "{serviceId}"
```

4. Wait for verification to complete

Wait for the verification to be successful, by making sure the `state` is `verified`
in the PrivateDnsConfiguration for the endpoint service.

#### Creating VPC Endpoint

For the controller to create the VPC endpoints it needs to compute the VPC and
the subnets.

The subnets that should be used depend on the AZs that are available for the endpoint
service. To figure out which AZs are supported by the service, the controller
needs to describe the endpoint service from the context of the hub account.

https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcEndpointServices.html
```
ec2.DescribeVpcEndpointServices
  ServiceName:
  - "{serviceName}"
```

The response includes a list of avaialabilty zones. These are the friendly names
for the AZs in hub account.
NOTE: this list can be different from the cluster account as friendly names are
different in different accounts.

The controller filters the list of subnets using the AZs. Using subnets in hub
account for all available AZs allows maximum availabity of connection.

The controller can then create VPC endpoint using
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_CreateVpcEndpoint.html
```
ec2.CreateVpcEndpoint
  PrivateDnsEnabled: True
  ServiceName: "{serviceName}"
  SubnetId:
  - subnet-1
  - subnet-2 or more
  VpcId: "{computed VPC id from subnets}"
  VpcEndpointType: Interface
```

The controller stores the VPC endpoint Id to the ClusterDeployment.

#### Waiting for VPC Endpoint approval

The endpoint service is allowed to automatically approve any endpoint created by
the allowed IAM principals. So the controller doesn't need to take additional
steps but since approval can take some time, it needs to wait for approval.

The controller can wait for approval using
https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeVpcEndpoints.html
```
ec2.DescribeVpcEndpoints
  VpcEndpointId:
  - "{endpoint Id}"
```

The controller ensures that the state of the VPC endpoint is `Available`.

#### Failed installations

Since the installations can fail, the endpoint service pointed to NLB will prevent
it's deletion and therefore to retry installation using the destroy the cluster and
retry again requires deleting the endpoint, endpoint service and in that order.

Currently when an installation fails, the new ClusterProvision uses the idenitfiers
from the old provision to destroy the cluster and then continue with trying to
install the cluster.

So when the controller decides that it now can begin adding resources to the cloud
i.e. when the ClusterDeployment has the cluster metadata set, it adds an annotation
`externalresources.finalizer.hive.openshift.io/aws-private-link-controller` to the
ClusterDeployment. The value of the annotation is the current InstallAttempt of the
ClusterDeployment.

The installmanager waits for all annotations of `externalresources.finalizer.hive.openshift.io/`
prefixes to greater than or equal to current InstallAttempt before continuing with the
destroying the cluster.

When the controller sees that the annotation exists but the values don't match, it uses the information
on the existing ClusterDeployment to cleanup all the resources it manages and sets the value
to the current InstallAttempt.

#### Reporting status on ClusterDeployment

Open Question: does is makes sense to add them to ClusterProvision

The controller adds progress reports and failures to the condition `AWSPrivateLinkReady`.
Some of reason the controller can use are:

"WaitingForClusterMetadata"
"InprogressCreateingVPCEndpointService"
"WaitingForDomainVerification"
"InprogressCreatingVPCEndpoint"
"WaitingForVPCEndpointApproval"
"VPCEndpointOk"
"Failed*"

### Control plane configuration at install time

#### Serving certificates

InstallManager should support setting up the serving certificates for apiserver
at install time.

1. Create APIServer object
2. Transform the specified Secret objects to openshift-config namespace

The install manager than uses the same code-flow as the manifests to add these
manifests during installation.

#### APIOverrideURL

APIOverrideURL is the endpoint used by hive to communicate with cluster for all
day-2 operations. Supporting this same behavior during installation allows the
installer to use the Private Link without any modifications.

Instructions for using non-default API server endpoint is described in RFE-438.

Run ignition-configs target to create the kubeconfig
```
openshift-install create ignition-configs
```
make sure the admin kubeconfig is stored or updated on the ClusterDeployment so
that user doesn't get the amended to include the override URL.
Update the `clusters[0].cluster` entry to appropiate server ans trust bundle.

```
openshift-install create cluster
```

### Risks and Mitigations


### Test Plan

## Alternatives

### EC2instance in user account to run installer

### installer that can create infrastructure without waiting

### New managed NLB instead of using installer created one

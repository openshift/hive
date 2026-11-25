# PrivateLink Controller

## Overview

Customers often do not want the services of their OpenShift cluster to be
publicly available on the Internet. The OpenShift Installer allows creating
clusters that have their services published only on the internal network by
setting `publish: Internal` in the install-config.yaml.

Since Hive is usually running outside the network where the cluster is being
deployed, special consideration must be taken to ensure it has access to the
cluster's API server. While there are multiple ways to create publicly
accessible endpoints that are limited such that only Hive can access them,
these are generally not acceptable by security focused customers.

Cloud providers usually have a method that allows accessing private services
from another network using the provider's internal networking instead of going
over the public Internet. The PrivateLink controller leverages these features
to enable Hive to access the APIs of these private clusters without exposing them
to the Internet.

## Architecture

The PrivateLink controller has been designed to enable a hive cluster to exist
on one platform while it deploys private clusters on any of the supported
platforms. It accomplishes this by breaking the functionality into two
segments (actuators):

### Hub Actuator

This actuator creates the necessary resources on the cloud provider that Hive is
running on. Its job is to provide a DNS record, and any other necessary resources,
to enable the Hive cluster to resolve the cluster's API to the Endpoint created
by the Link Actuator.

Supported Platforms: [AWS](#aws-hub-actuator)

### Link Actuator

This actuator creates the necessary resources on the cloud provider that the
private cluster is being provisioned into. Its job is to create an Endpoint,
and any other necessary resources, to enable the Hive cluster to connect to the
cluster's API without enabling public access.

Supported Platforms: [GCP](#gcp-link-actuator)

Note: Support for AWS private clusters is still managed by the original [AWS
PrivateLink Controller](awsprivatelink.md). The goal is to eventually merge its
functionality into this PrivateLink Controller.

## AWS Hub Actuator Configuration

This actuator is used when the Hive cluster exists in the AWS platform. It will
ensure a new DNS zone exists for the private cluster and create a DNS record
which resolves to the Endpoint created by the Link Actuator. This zone will
then be attached to the associatedVPCs configured in the hiveconfig.

Note: The AWS Hub Actuator currently uses the same HiveConfig parameters as the
original AWS PrivateLink controller. This is to avoid configuration drift
between the two controllers until they can be merged together.

### Configure the primary AWS credentialsSecretRef

The Hub Actuator needs an AWS Service Account in order to manage the DNS
Zone and records. The credentials for this service account should be stored in
a secret in the Hive namespace. This secret can then be configured via the
`credentialsSecretRef` parameter in the hiveconfig.

```yaml
## hiveconfig
spec:
  awsPrivateLink:
  ## credentialsSecretRef points to a secret with permissions to create
  ## resources in account where the inventory of VPCs exist.
  credentialsSecretRef:
    name: < hub-account-credentials-secret-name >
```

### Configure the AWS associatedVPCs

The Hub Actuator needs to associate the DNS Zone with the Hive cluster's VPC in
order for Hive to be able to resolve the API. This can be configured via the
`associatedVPCs` parameter in the hiveconfig. Each VPC in this list will be
associated with the DNS Zone. By default, they are expected to be in the same
account as configured by the primary service account above. This can be
overridden using the corresponding `credentialsSecretRef` fields.

When creating the DNS Zone, the Hub Actuator must attach it to at least one
network that exists in the same account as the DNS Zone. For AWS private
clusters, this is the network the endpoint is created in. However, this isn't
possible for other platforms. The Actuator needs another way to choose which
network to use. For this reason, this list should include at least one VPC
where the `credentialsSecretRef` parameter is either empty or has a value equal
to that of `spec.awsPrivateLink.credentialsSecretRef`.

```yaml
## hiveconfig
spec:
  awsPrivateLink:
    ## this is a list of VPC where various Hive clusters exists.
    associatedVPCs:
    - region: region-hive1
      vpcID: vpc-hive1
    - region: region-hive2
      vpcID: vpc-hive2
      credentialsSecretRef:
        name: < optional credentials that have access to account where Hive2 VPC exists>
```

## GCP Link Actuator Configuration

This actuator is used when the clusterDeployment specifies the GCP platform and
has `spec.platform.gcp.privateServiceConnect.enabled` set to true. It will
ensure an Endpoint and associated Service Attachment exist to enable access to
the cluster's API.

### Configure the primary GCP credentialsSecretRef

The Link Actuator needs a GCP Service Account in order to manage the Endpoint.
The credentials for this service account should be stored in a secret in the
Hive namespace. This secret can then be configured via the
`credentialsSecretRef` parameter in the hiveconfig.

```yaml
## hiveconfig
spec:
  privateLink:
    gcp:
      ## credentialsSecretRef points to a secret with permissions to create
      ## resources in account where the inventory of VPCs exist.
      credentialsSecretRef:
        name: < link-account-credentials-secret-name >
```

This service account requires the following permissions:

- Endpoint Address
    - compute.addresses.create
    - compute.addresses.createInternal
    - compute.addresses.delete
    - compute.addresses.deleteInternal
    - compute.addresses.get
    - compute.addresses.list
    - compute.instances.update
    - compute.regionOperations.get
    - compute.subnetworks.get
- Endpoint
    - compute.addresses.use
    - compute.forwardingRules.create
    - compute.forwardingRules.delete
    - compute.forwardingRules.get
    - compute.forwardingRules.pscCreate
    - compute.forwardingRules.pscDelete
    - compute.networks.use
    - compute.regionOperations.get
    - compute.subnetworks.use
    - servicedirectory.namespaces.create
    - servicedirectory.services.create
    - servicedirectory.services.delete


### Configure the GCP endpointVPCInventory

The Link Actuator needs to know where to create the cluster's Endpoint. This
can be configured in the `spec.privateLink.gcp.endpointVPCInventory` parameter
in the hiveconfig. When creating the endpoint, the actuator will choose the
subnet from this list that is in the same region and has the least number of
addresses provisioned in the cloud provider.

1. Create a network for the PrivateLink Controller to use when creating
endpoints.

1. Create one or more subnets in the network for each region that private
clusters will be created in.

1. Make sure all the Hive environments (Hive VPCs) have network reachability to
these subnets using peering, transit gateways, VPNs, etc.

1. Update the HiveConfig to enable these subnets.

```yaml
## hiveconfig
spec:
  privateLink:
    gcp:
      endpointVPCInventory:
      - network: network-1
        subnets:
        - region: us-east1
          subnet: subnet1
        - region: us-east2
          subnet: subnet2
```

## Deploying a PrivateLink cluster on AWS

Support for AWS private clusters is still managed by the original [AWS
PrivateLink Controller](awsprivatelink.md). The goal is to eventually merge its
functionality into this PrivateLink Controller.

## Deploying a PrivateLink cluster on GCP

Once Hive has been configured to support PrivateLink GCP clusters, you can
deploy a cluster by setting `privateServiceConnect.enabled` to true on the
clusterDeployment. This is only supported in regions where Hive has been
configured with at least one subnet in [endpointVPCInventory](#configure-the-gcp-endpointvpcinventory)
with the same region. The Link Actuator uses the service account specified in
the spec.platform.gcp.credentialsSecretRef parameter of the clusterDeployment
when managing the Service Attachment, Subnet, and Firewall resources.


```yaml
## clusterDeployment
spec:
  platform:
    gcp:
      privateServiceConnect:
        enabled: true
```

By default, the Link Actuator will create a subnet and associated firewall in
the clusters's VPC to be used when creating the Service Attachment. The default
CIDR of the subnet can be overridden with the `serviceAttachment.subnet.cidr`
parameter

```yaml
## clusterDeployment
spec:
  platform:
    gcp:
      privateServiceConnect:
        serviceAttachment:
          subnet:
            cidr: 192.168.1.0/30
```

Alternatively, a pre-existing subnet can be specified and the Link Actuator
will use it instead of creating a new one. This subnet must have a purpose of
`PRIVATE_SERVICE_CONNECT` and have routing and firewall rule(s) enabling access
to the api-internal load balancer. Specifying an existing subnet is required
when using BYO VPC. The host project name must also be specified when using
Shared VPC.

```yaml
## clusterDeployment
spec:
  platform:
    gcp:
      privateServiceConnect:
        serviceAttachment:
          subnet:
            existing:
              name: psc-subnet-name
              project: shared-vpc-host-project-name
```
The clusterDeployment service account requires the following permissions in addition to those required to install a cluster:

- Firewall
    - compute.firewalls.create
    - compute.firewalls.delete
    - compute.firewalls.get
    - compute.networks.updatePolicy
    - compute.regionOperations.get
- Service Attachment
    - compute.forwardingRules.get
    - compute.regionOperations.get
    - compute.serviceAttachments.create
    - compute.serviceAttachments.delete
    - compute.serviceAttachments.get
    - compute.subnetworks.get
- Subnet
    - compute.networks.get
    - compute.regionOperations.get
    - compute.subnetworks.create
    - compute.subnetworks.delete
    - compute.subnetworks.get
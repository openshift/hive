# hiveutil

**NOTE: `hiveutil` is an internal utility for use by hive developers and
hive itself. It is not supported for general use.**

The `hiveutil` CLI offers several commands to help manage clusters with Hive.

To build the `hiveutil` binary, run `make build-hiveutil`.

### Create Cluster

The `create-cluster` command generates a `ClusterDeployment` and submits it to the Hive cluster using your current kubeconfig.

To view what `create-cluster` generates, *without* submitting it to the API server, add `-o yaml` to `create-cluster`. If you need to make any changes not supported by `create-cluster` options, the output can be saved, edited, and then submitted with `oc apply`. This is also a useful way to generate sample yaml.

`--release-image` can be specified to control which OpenShift release image to use.

#### Create Cluster on AWS

Credentials will be read from your AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables. If the environment variables are missing or empty, then `create-cluster` will look for creds at `~/.aws/credentials`. Alternatively you can specify an AWS credentials file with `--creds-file`.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=aws mycluster
```

#### Create Cluster on Azure

Credentials will be read from either `~/.azure/osServicePrincipal.json`, the contents of the `AZURE_AUTH_LOCATION` environment variable, or the value provided with the `--creds-file` parameter (in increasing order of preference). The format for the credentials used for installation/uninstallation follows the same format used by the OpenShift installer:

```
{
  "subscriptionId": "azure-subscription-uuid-here",
  "clientId": "client-id-for-service-principal",
  "clientSecret": "client-secret-for-service-principal",
  "tenantId": "tenant-uuid-here"
}
```

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=azure --azure-base-domain-resource-group-name=myresourcegroup --azure-cloud-name=AzurePublicCloud mycluster
```

NOTE: `--azure-cloud-name=AzurePublicCloud` specifies the Azure Cloud in which the cluster will be created e.g. `AzurePublicCloud` or `AzureUSGovernmentCloud`.

NOTE: For deprovisioning a cluster `hiveutil` will use creds from `~/.azure/osServiceAccount.json` or the `AZURE_AUTH_LOCATION` environment variable (with the environment variable prefered).

#### Create Cluster on GCP

Credentials will be read from either `~/.gcp/osServiceAccount.json`, the contents of the `GOOGLE_CREDENTIALS` environment variable, or the value provided with the `--creds-file` parameter (in increasing order of preference). GCP credentials can be created by:

 1. Login to GCP console at https://console.cloud.google.com/
 1. Create a service account with the owner role.
 1. Create a key for the service account.
 1. Select JSON for the key type.
 1. Download resulting JSON file and save to `~/.gcp/osServiceAccount.json`.

```bash
bin/hiveutil create-cluster --base-domain=mydomain.example.com --cloud=gcp mycluster
```

NOTE: For deprovisioning a cluster, `hiveutil` will use creds from `~/.gcp/osServiceAccount.json` or the `GOOGLE_CREDENTIALS` environment variable (with the environment variable prefered).

#### Create Cluster on vSphere

Set credentials/connection information in the following environment variables. `GOVC_USERNAME` should hold the vSphere username, `GOVC_PASSWORD` should be set to the vSphere user's password. If the vCenter instance is using self-signed certificates or is otherwise untrusted by the system being used to connect to vCenter, `GOVC_TLS_CA_CERTS` should be set to the path of a file containing the CA certificate for the vCenter instance. 

The following parameters are required and must be provided via environment variable or command line parameter:

| Environment Variable | Command line parameter                    |
| -------------------- | ----------------------------------------- |
| `GOVC_USERNAME `     | Must be provided as environment variable. |
| `GOVC_PASSWORD`      | Must be provided as environment variable. |
| `GOVC_TLS_CA_CERTS`  | `--vsphere-ca-certs`                      |
| `GOVC_DATACENTER`    | `--vsphere-datacenter`                    |
| `GOVC_DATASTORE`     | `--vsphere-default-datastore`             |
| `GOVC_HOST`          | `--vsphere-vcenter`                       |


```bash
bin/hiveutil create-cluster --cloud=vsphere --vsphere-vcenter=vcenter.example.com --vsphere-datacenter=dc1 --vsphere-default-datastore=ds1 --vsphere-api-vip=192.168.10.10 --vsphere-ingress-vip=192.168.10.11 --vsphere-cluster=devel --vsphere-network="VM Network" --vsphere-folder=/dc1/vm/mycluster --vsphere-ca-certs="/tmp/cert1.crt:/tmp/cert2.crt" --base-domain vmware.hive.example.com mycluster
```

#### Create Cluster on OpenStack

Credentials will be read from `~/.config/openstack/clouds.yaml` or `/etc/openstack/clouds.yaml`. An example file looks like:
```yaml
clouds:
  mycloud:
    auth:
      auth_url: https://test.auth.url.example.com:13000/v3
      username: "test-user"
      password: "secret-password"
      project_id: 97aa533a6f094222ae76f097e2eb1df4
      project_name: "openshift"
      user_domain_name: "example.com"
    region_name: "regionOne"
    interface: "public"
    identity_api_version: 3
```

```bash
bin/hiveutil create-cluster --cloud=openstack --openstack-api-floating-ip=192.168.1.2 --openstack-cloud=mycloud mycluster
```

#### Create Cluster on IBM Cloud

The IBM Cloud API key will be read from an `IC_API_KEY` environment variable. An [IBM Cloud credential manifests](./using-hive.md#ibm-cloud-credential-manifests) directory containing cloud credential secrets for OpenShift components must be provided.

```bash
bin/hiveutil create-cluster --cloud=ibmcloud --region="us-south" --base-domain=ibm.hive.openshift.com --manifests=/path/to/manifests/ --credentials-mode-manual mycluster
```

### Cluster Pools

Create a [ClusterPool](./clusterpools.md):

```bash
bin/hiveutil clusterpool create-pool -n hive --cloud=aws --creds-file ~/.aws/credentials --image-set openshift-46 --pull-secret-file ~/.pull-secret --region us-east-1 --size 5 test-pool
```

Claim a ClusterDeployment from a [ClusterPool](./clusterpools.md):

```bash
bin/hiveutil clusterpool claim -n hive test-pool username-claim
```

### AWS PrivateLink

To create an AWS cluster using [PrivateLink](./awsprivatelink.md), the following steps could be followed:

Initialize AWS PrivateLink settings for Hive:

```bash
bin/hiveutil awsprivatelink enable --dns-record-type Alias
```

Explanation:
1) This command creates a Secret with AWS hub account credentials extracted from the environment.
2) It adds a reference to the Secret in `HiveConfig.spec.awsPrivateLink.credentialsSecretRef`. 
3) The active cluster's VPC is added to `HiveConfig.spec.awsPrivateLink.associatedVPCs`.
4) `HiveConfig.spec.awsPrivateLink.dnsRecordType` is set to Alias.

Create an endpoint VPC with private subnets:

```bash
# Could be different from the active cluster's region
export REGION=us-east-1
export STACK=my-stack-name
export TEMPLATE=file://hack/awsprivatelink/vpc.cf.yaml
export AZCOUNT=3
# Should not overlap with the CIDR of the active cluster's VPC
export CIDR=10.1.0.0/16

aws --region $REGION cloudformation create-stack --stack-name $STACK --template-body $TEMPLATE --parameters ParameterKey=AvailabilityZoneCount,ParameterValue=$AZCOUNT ParameterKey=VpcCidr,ParameterValue=$CIDR
aws --region $REGION cloudformation wait stack-create-complete --stack-name $STACK
export PRIVSUBNETIDS=$(aws cloudformation describe-stacks --region $REGION --stack-name $STACK --query 'Stacks[0].Outputs[?OutputKey==`PrivateSubnetIds`].OutputValue | [0]' --output text)
export VPCID=$(aws cloudformation describe-stacks --region $REGION --stack-name $STACK --query 'Stacks[0].Outputs[?OutputKey==`VpcId`].OutputValue | [0]' --output text)
```

Set up the endpoint VPC for PrivateLink:

```bash
bin/hiveutil awsprivatelink endpointvpc add $VPCID --subnet-ids $PRIVSUBNETIDS --region $REGION
```

Explanation:
1) This command sets up networking elements on AWS to allow traffic between the endpoint VPC and each associated VPC.
2) It adds the endpoint VPC to `HiveConfig.spec.awsPrivateLink.endpointVPCInventory`.

Create an AWS cluster using PrivateLink:

```bash
export CLUSTERNAME=my-cluster-name
export BASEDOMAIN=my.base.domain.com

bin/hiveutil create-cluster $CLUSTERNAME --base-domain $BASEDOMAIN --region $REGION --aws-private-link --internal
```

Remove the cluster after usage:

```bash
oc delete cd $CLUSTERNAME
```

Remove the endpoint VPC after the CD is gone:

```bash
bin/hiveutil awsprivatelink endpointvpc remove $VPCID
```

Explanation:
1) This command tears down the networking elements that were set up using `bin/hiveutil awsprivatelink endpointvpc add`.
2) It also removes the endpoint VPC from `HiveConfig.spec.awsPrivateLink.endpointVPCInventory`.

Delete the CloudFormation stack:

```bash
aws --region $REGION cloudformation delete-stack --stack-name $STACK
```

Disable AWS PrivateLink in HiveConfig:

```bash
bin/hiveutil awsprivatelink disable
```

Explanation:
1) This command removes the AWS hub account credentials Secret created with `bin/hiveutil awsprivatelink enable` from Hive's namespace.
2) It empties `HiveConfig.spec.awsPrivateLink`, restoring HiveConfig to its state before configuring PrivateLink.

### Other Commands

To see other commands offered by `hiveutil`, run `hiveutil --help`.

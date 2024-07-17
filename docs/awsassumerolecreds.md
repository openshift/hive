# AWS AssumeRole credentials for clusters

Currently ClusterDeployments for AWS require customers to provide a Secret with static credentials.
AWS best practices recommend against using static credentials in favour of temporary credential
sources like IAM roles. The requirement for static credentials is enforced by OpenShift clusters,
but in certain cases these requirements can be removed.

OpenShift clusters with Manual AWS credential management as outlined in [AWS STS Provisioning](./aws-sts-provisioning.md)
support using temporary credentials for installing the clusters.

To allow use AssumeRole credentials, the customer must trust AWS IAM entities like IAM roles in service provider's
AWS account to assume the role specified. The workflow looks like,

`Some credentials source` -> assume role -> `platform service provider IAM role` -> assume role -> `customer IAM role`

This allows customers to specify only the IAM role in ClusterDeployment and not provide static credentials.

## Setting up AWS Service Provider credentials for Hive

As mentioned above, the customers trust service provider IAM role to assume the customer's IAM role.
So Hive must have credentials to assume the service provider IAM role.

### Create AWS Service Provider credentials secret

Let's create a Secret in `hive` (target namespace in HiveConfig) with credentials,

```console
$ cat aws-service-provider-config
[profile source]
aws_access_key_id = XXXXXX
aws_secret_access_key = XXXXX
role_arn = arn:aws:iam::123456:role/hive-aws-service-provider

$ oc create -n hive secret generic aws-service-provider-secret --from-file=aws_config=aws-service-provider-config
Secret created "aws-service-provider-secret"
```

### Update HiveConfig to use the AWS Service Provider secret

Let's update the HiveConfig to configure Hive with the credentials for AWS service provider IAM role,

```console
oc patch hiveconfig hive --type='json' -p='[{"op":"replace","path":"/spec/serviceProviderCredentialsConfig","value":{"aws":{"credentialsSecretRef": {"name": "aws-service-provider-secret"}}}}]'
```

The HiveConfig must look like,

```yaml
spec:
  serviceProviderCredentialsConfig:
    aws:
      credentialsSecretRef:
        name: aws-service-provider-secret
```

**WARNING:** Hive controllers will copy this secret to ClusterDeployment namespaces that need access to this secret
to install and uninstall the clusters.

## Setting up IAM trust from customer to service provider

Let's assume that the service provider IAM role is `arn:aws:iam::123456:role/hive-aws-service-provider`. The customer
must allow this role to assume the role they want to use for ClusterDeployment.

The assume role policy for the customer IAM role look like,

```console
$ cat customer-assume-role.json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "AWS": "arn:aws:iam::123456:role/hive-aws-service-provider"
            },
            "Action": "sts:AssumeRole"
            "Condition": {"StringEquals": {"sts:ExternalId": "Unique ID Assigned by Platform"}}
        }
    ]
}
```

Let's create an IAM role,

```sh
aws iam create-role --role-name customer-openshift-role --assume-role-policy-document file://customer-assume-role.json
```

Now add the required permissions to the IAM role `customer-openshift-role`. One way is to update the IAM role's
inline IAM policy like below,

Here `openshift-permissions.json` is an IAM policy that includes all the required permissions for creating, managing,
and deleting OpenShift clusters.

```sh
aws iam put-role-policy --role-name customer-openshift-role --policy-name customer-openshift-role-policy --policy-document file://openshift-permissions.json
```

## Creating ClusterDeployment with AssumeRole Credentials

1. Follow the steps outlined in [AWS STS Provisioning](./aws-sts-provisioning.md) to prepare the
    ClusterDeployment with install-config.yaml, manifests, etc.

2. Update the ClusterDeployment `aws` platform to specify the IAM role.

    ```yaml
    spec:
      platform:
        aws:
          credentialsAssumeRole:
            roleARN: arn:aws:iam::789012:role/customer-openshift-role
            externalID: "Unique ID Assigned by Platform"
    ```

    Make sure `credentialsSecretRef` field is not set in the ClusterDeployment `aws` platform.

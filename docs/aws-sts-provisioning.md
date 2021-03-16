# Provisioning AWS STS Clusters

It is possible to use Hive to provision clusters configured to use Amazon's Security Token Service, where cluster components use short lived credentials that are rotated frequently. This feature was added to the in-cluster Cloud Credential Operator in 4.7, see documentation [here](https://docs.openshift.com/container-platform/4.7/authentication/managing_cloud_provider_credentials/cco-mode-sts.html).

At present Hive does not automate the STS setup, rather we assume the user configures STS components manually and provides information to Hive. The following instructions refer to the 'ccoctl' coming in OpenShift 4.8 with the Cloud Credential Operator. This tool is not yet released but development is underway [here](https://github.com/openshift/cloud-credential-operator) and can be compiled from source.

## Setup STS Infrastructure

```bash
$ ccoctl create key-pair
$ ccoctl create identity-provider --name-prefix mystsprefix --public-key-file serviceaccount-signer.public --region us-east-1
```

## Extract Credentials Requests from Desired OpenShift Release Image

These vary from release to release, so be sure to use the same release image here as you plan to use for your ClusterDeployment:

```bash
$ mkdir credrequests/
$ oc adm release extract quay.io/openshift-release-dev/ocp-release:4.7.1-x86_64 --credentials-requests --cloud=aws > credrequests/credrequests.yaml
```

## Generate AWS IAM Roles

```bash
$ create iam-roles --credentials-requests-dir /home/dgoodwin/go/src/github.com/openshift/cloud-credential-operator/sts/credrequests/ --identity-provider-arn arn:aws:iam::125931421481:oidc-provider/dgoodsts2-oidc.s3.us-east-1.amazonaws.com --name-prefix dgoodsts2 --region us-east-1
```

## Create Credentials Secret Manifests

Hive allows passing in arbitrary Kubernetes resource manifests to pass through to the install process. We will leverage this to inject the Secrets and configuration required for an STS cluster.

ccoctl will soon have a command to generate these Secrets but this is not implemented yet. For now you can follow the [manual STS documentation](https://docs.openshift.com/container-platform/4.7/authentication/managing_cloud_provider_credentials/cco-mode-sts.html) for how to create each Secret manually for the CredentialsRequests in your release image.

Use the Role ARN's printed by `ccoctl create iam-roles`.

Example:

```yaml
apiVersion: v1
stringData:
  credentials: |-
    [default]
    role_arn = arn:aws:iam::125931421481:role/mystsprefix-openshift-image-registry-installer-cloud-credentials
    web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
kind: Secret
metadata:
  name: installer-cloud-credentials
  namespace: openshift-image-registry
type: Opaque
```


### Create Authentication Manifest

In the same directory as your Secret manifets, add another to configure the OpenShift Authentication operator to use the S3 OIDC provider created by `ccoctl create identity-provider`.

```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  serviceAccountIssuer: https://mystsprefix-oidc.s3.us-east-1.amazonaws.com
```

May also soon be automated by ccoctl.

## Create Hive ClusterDeployment

Create a ClusterDeployment normally with the following changes:

  1. Create a Secret for your private service account signing key created with ccoctl key-pair above: `kubectl create secret generic bound-service-account-signing-key --from-file=bound-service-account-signing-key.key=serviceaccount-signer.private`
  1. Create a ConfigMap with keys for each IAM Role Secret. Each key will be the file name provided to the installer: `kubectl create configmap cluster-manifests --from-file=manifest1.yaml=manifets/secret1.yaml --from-file=manifest2.yaml=manifests/secret2.yaml`
  1. In your InstallConfig set `credentialsMode: Manual`
  1. In your ClusterDeployment set `spec.platform.aws.sts.serviceAccountIssuerKeySecretRef.name` to point to the Secret created above. (bound-service-account-signing-key)
  1. In your ClusterDeployment set `spec.spec.provisioning.manifestsConfigMapRef` to point to the ConfigMap created above. (cluster-manifests)
  1. Create your ClusterDeployment + InstallConfig to provision your STS cluster.

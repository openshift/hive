# Provisioning AWS STS Clusters

It is possible to use Hive to provision clusters configured to use Amazon's Security Token Service, where cluster components use short lived credentials that are rotated frequently, and the cluster does not have an admin level AWS credential. This feature was added to the in-cluster OpenShift components in 4.20, see documentation [here](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/authentication_and_authorization/managing-cloud-provider-credentials#cco-short-term-creds).

At present Hive does not automate the STS setup, rather we assume the user configures STS components manually and provides information to Hive. The following instructions refer to the `ccoctl` tool. This tool can be extracted from the OpenShift release image. See steps below.

## Extract Credentials Requests from Desired OpenShift Release Image

CredentialsRequests vary from release to release, so be sure to use the same release image here as you plan to use for your ClusterDeployment.

`$RELEASE_IMAGE` should be a recent and supported OpenShift release image that you want to deploy in your cluster.
Please refer to the [support matrix](../README.md#support-matrix) for compatibilities.

A sample release image would be `RELEASE_IMAGE=quay.io/openshift-release-dev/ocp-release:${RHOCP_version}-${Arch}`

Where `RHOCP_version` is the OpenShift version (e.g `4.10.0-fc.4` or `4.9.3`) and the `Arch` is the architecture type (e.g `x86_64`)

```bash
$ mkdir credrequests/
$ oc adm release extract $RELEASE_IMAGE --credentials-requests --cloud=aws --to=./credrequests
```

## Extract the `ccoctl` binary from the release image.

`$PULL_SECRET` should be the path to your pull secret required for pulling the release image.

```
CCO_IMAGE=$(oc adm release info --image-for='cloud-credential-operator' ${RELEASE_IMAGE}) && oc image extract ${CCO_IMAGE} --file='/usr/bin/ccoctl' --registry-config=${PULL_SECRET}
chmod u+x ccoctl
```

## Setup STS Infrastructure

Create AWS resources using the ccoctl tool (you will need aws credentials with sufficient permissions). The command below will generate public/private ServiceAccount signing keys, create the S3 bucket (with public read-only access), upload the OIDC config into the bucket, set up an IAM Identity Provider that trusts that bucket configuration, and create IAM Roles for each AWS CredentialsRequest extracted above. It will also dump the files needed by the installer in the `_output` directory. Installation secret manifests will be found within `_output/manifests`.
```
./ccoctl aws create-all --name <aws_infra_name> --region <aws_region> --credentials-requests-dir ./credrequests --output-dir _output/
```

Hive allows providing arbitrary Kubernetes resource manifests to pass through to the install process. We will leverage this to inject the Secrets and configuration required for an STS cluster by creating a secret containing these manifests in the steps below. The manifests secret will be referenced from the ClusterDeployment.

## Create Hive ClusterDeployment

Create a ClusterDeployment normally with the following changes:

  1. Create a Secret for your private service account signing key created with `ccoctl aws create-all` above.
  ```
  kubectl create secret generic bound-service-account-signing-key --from-file=bound-service-account-signing-key.key=_output/serviceaccount-signer.private
  ```
  1. Create a Secret for your installer manifests (credential role Secrets, Authentication config)
  ```
  kubectl create secret generic cluster-manifests --from-file=_output/manifests/
  ```
  1. In your InstallConfig set `credentialsMode: Manual`
  1. In your ClusterDeployment set `spec.boundServiceAccountSigningKeySecretRef.name` to point to the Secret created above (`bound-service-account-signing-key`).
  1. In your ClusterDeployment set `spec.provisioning.manifestsSecretRef` to point to the Secret created above (`cluster-manifests`).
  1. Create your ClusterDeployment + InstallConfig to provision your STS cluster.

## Note: Cleanup AWS resources after uninstalling the cluster
Make sure you clean up the following resources after you uninstall your cluster. To delete resources created by ccoctl, run
```bash
$ ./ccoctl aws delete --name=<name> --region=<aws-region>
```
where name is the name used to tag and account any cloud resources that were created, and region is the aws region in which cloud resources were created.

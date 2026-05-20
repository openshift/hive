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
  1. Create a Secret for your installer manifests (credential role Secrets, Authentication config).
  The recommended approach is `--from-file` pointed at the `ccoctl` output directory, which
  automatically preserves the original filenames as secret keys:
  ```
  kubectl create secret generic cluster-manifests --from-file=_output/manifests/
  ```

  > **WARNING: Authentication CR key name requirement**
  >
  > The secret key for the Authentication CR **must** be exactly
  > `cluster-authentication-02-config.yaml`. During bootstrap, the
  > kube-apiserver render step reads this manifest from a hardcoded path
  > (`/assets/manifests/cluster-authentication-02-config.yaml`). If the
  > key name in the manifest secret differs from this, the file will not
  > be found and the kube-apiserver will silently start with the default
  > `serviceAccountIssuer` (`https://kubernetes.default.svc`) instead of
  > your custom S3 OIDC issuer. This causes `machine-api-controllers` to
  > receive tokens with the wrong issuer, and AWS STS rejects them with
  > `InvalidIdentityToken: Token issuer does not match provider`. Workers
  > will never provision and the install will time out.
  >
  > Using `--from-file=_output/manifests/` as shown above preserves the
  > canonical filename automatically. If you create the secret manually
  > (e.g. with `stringData` in YAML), ensure the Authentication CR entry
  > uses the key `cluster-authentication-02-config.yaml`.
  >
  > Other credential manifests (operator Secrets for machine-api, ingress,
  > image-registry, etc.) are not affected by this requirement — they are
  > applied by their Kubernetes GVK and content, not by filename.

  1. In your InstallConfig set `credentialsMode: Manual`
  1. In your ClusterDeployment set `spec.boundServiceAccountSigningKeySecretRef.name` to point to the Secret created above (`bound-service-account-signing-key`).
  1. In your ClusterDeployment set `spec.provisioning.manifestsSecretRef` to point to the Secret created above (`cluster-manifests`).
  1. Create your ClusterDeployment + InstallConfig to provision your STS cluster.

## Troubleshooting

### Install times out with `InvalidIdentityToken`

If your STS cluster install times out and `machine-api-controllers` logs show:

```
error assuming role: InvalidIdentityToken: Token issuer does not match provider
```

The kube-apiserver is likely using the default `serviceAccountIssuer` instead of your custom S3 OIDC issuer.

**Check the issuer on the running cluster:**

```bash
oc get authentication cluster -o jsonpath='{.spec.serviceAccountIssuer}'
```

If this returns your S3 OIDC URL (e.g. `https://<name>-oidc.s3.<region>.amazonaws.com`) but the
kube-apiserver started with `https://kubernetes.default.svc`, the Authentication CR manifest was
not picked up during bootstrap.

**Verify the manifest secret key names:**

```bash
oc get secret cluster-manifests -n <namespace> -o jsonpath='{range .data}{@.key}{"\n"}{end}'
```

Look for `cluster-authentication-02-config.yaml` as an exact key name. If the Authentication CR
is stored under a different key (e.g. `00-cluster-authentication-config.yaml` or
`authentication.yaml`), recreate the secret with the correct key name.

The simplest fix is to recreate the manifest secret using `--from-file` pointed at the `ccoctl`
output directory:

```bash
oc delete secret cluster-manifests -n <namespace>
oc create secret generic cluster-manifests -n <namespace> --from-file=_output/manifests/
```

## Note: Cleanup AWS resources after uninstalling the cluster
Make sure you clean up the following resources after you uninstall your cluster. To delete resources created by ccoctl, run
```bash
$ ./ccoctl aws delete --name=<name> --region=<aws-region>
```
where name is the name used to tag and account any cloud resources that were created, and region is the aws region in which cloud resources were created.
